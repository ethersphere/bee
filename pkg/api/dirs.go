// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
)

const (
	contentTypeTar = "application/x-tar"
)

const (
	manifestRootPath                      = "/"
	manifestWebsiteIndexDocumentSuffixKey = "website-index-document"
	manifestWebsiteErrorDocumentPathKey   = "website-error-document"
	manifestEntryMetadataContentTypeKey   = "Content-Type"
	manifestEntryMetadataFilenameKey      = "Filename"
)

// dirUploadHandler uploads a directory supplied as a tar in an HTTP request
func (s *server) dirUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	if r.Body == http.NoBody {
		logger.Error("dir upload, request has no body")
		jsonhttp.BadRequest(w, "could not validate request")
		return
	}
	contentType := r.Header.Get(contentTypeHeader)
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		logger.Errorf("dir upload, invalid content-type")
		logger.Debugf("dir upload, invalid content-type err: %v", err)
		jsonhttp.BadRequest(w, "could not validate request")
		return
	}

	var dReader dirReader
	switch mediaType {
	case contentTypeTar:
		dReader = &tarReader{r: tar.NewReader(r.Body)}
	case multiPartFormData:
		dReader = &multipartReader{r: multipart.NewReader(r.Body, params["boundary"])}
	default:
		logger.Error("dir upload, invalid content-type for directory upload")
		jsonhttp.BadRequest(w, "could not validate request")
		return
	}
	defer r.Body.Close()

	tag, created, err := s.getOrCreateTag(r.Header.Get(SwarmTagHeader))
	if err != nil {
		logger.Debugf("dir upload: get or create tag: %v", err)
		logger.Error("dir upload: get or create tag")
		jsonhttp.InternalServerError(w, "cannot get or create tag")
		return
	}

	// Add the tag to the context
	reference, err := storeDir(
		sctx.SetTag(r.Context(), tag),
		requestEncrypt(r),
		dReader,
		s.logger,
		requestPipelineFn(s.storer, r),
		loadsave.New(s.storer, requestModePut(r), requestEncrypt(r)),
		r.Header.Get(SwarmIndexDocumentHeader),
		r.Header.Get(SwarmErrorDocumentHeader),
		tag,
		created,
	)
	if err != nil {
		logger.Debugf("dir upload: store dir err: %v", err)
		logger.Errorf("dir upload: store dir")
		jsonhttp.InternalServerError(w, "could not store dir")
		return
	}
	if created {
		_, err = tag.DoneSplit(reference)
		if err != nil {
			logger.Debugf("dir upload: done split: %v", err)
			logger.Error("dir upload: done split failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
	}
	w.Header().Set(SwarmTagHeader, fmt.Sprint(tag.Uid))
	jsonhttp.OK(w, fileUploadResponse{
		Reference: reference,
	})
}

// storeDir stores all files recursively contained in the directory given as a tar
// it returns the hash for the uploaded manifest corresponding to the uploaded dir
func storeDir(
	ctx context.Context,
	encrypt bool,
	reader dirReader,
	log logging.Logger,
	p pipelineFunc,
	ls file.LoadSaver,
	indexFilename,
	errorFilename string,
	tag *tags.Tag,
	tagCreated bool,
) (swarm.Address, error) {
	logger := tracing.NewLoggerWithTraceID(ctx, log)

	dirManifest, err := manifest.NewDefaultManifest(ls, encrypt)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	if indexFilename != "" && strings.ContainsRune(indexFilename, '/') {
		return swarm.ZeroAddress, fmt.Errorf("index document suffix must not include slash character")
	}

	filesAdded := 0

	// iterate through the files in the supplied tar
	for {
		fileInfo, err := reader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("read tar stream: %w", err)
		}

		if !tagCreated {
			// only in the case when tag is sent via header (i.e. not created by this request)
			// for each file
			if estimatedTotalChunks := calculateNumberOfChunks(fileInfo.Size, encrypt); estimatedTotalChunks > 0 {
				err = tag.IncN(tags.TotalChunks, estimatedTotalChunks)
				if err != nil {
					return swarm.ZeroAddress, fmt.Errorf("increment tag: %w", err)
				}
			}
		}

		fileReference, err := p(ctx, fileInfo.Reader, fileInfo.Size)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("store dir file: %w", err)
		}
		logger.Tracef("uploaded dir file %v with reference %v", fileInfo.Path, fileReference)

		fileMtdt := map[string]string{
			manifestEntryMetadataContentTypeKey: fileInfo.ContentType,
			manifestEntryMetadataFilenameKey:    fileInfo.Name,
		}
		// add file entry to dir manifest
		err = dirManifest.Add(ctx, fileInfo.Path, manifest.NewEntry(fileReference, fileMtdt))
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("add to manifest: %w", err)
		}

		filesAdded++
	}

	// check if files were uploaded through the manifest
	if filesAdded == 0 {
		return swarm.ZeroAddress, fmt.Errorf("no files in tar")
	}

	// store website information
	if indexFilename != "" || errorFilename != "" {
		metadata := map[string]string{}
		if indexFilename != "" {
			metadata[manifestWebsiteIndexDocumentSuffixKey] = indexFilename
		}
		if errorFilename != "" {
			metadata[manifestWebsiteErrorDocumentPathKey] = errorFilename
		}
		rootManifestEntry := manifest.NewEntry(swarm.ZeroAddress, metadata)
		err = dirManifest.Add(ctx, manifestRootPath, rootManifestEntry)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("add to manifest: %w", err)
		}
	}

	storeSizeFn := []manifest.StoreSizeFunc{}
	if !tagCreated {
		// only in the case when tag is sent via header (i.e. not created by this request)
		// each content that is saved for manifest
		storeSizeFn = append(storeSizeFn, func(dataSize int64) error {
			if estimatedTotalChunks := calculateNumberOfChunks(dataSize, encrypt); estimatedTotalChunks > 0 {
				err = tag.IncN(tags.TotalChunks, estimatedTotalChunks)
				if err != nil {
					return fmt.Errorf("increment tag: %w", err)
				}
			}
			return nil
		})
	}

	// save manifest
	manifestReference, err := dirManifest.Store(ctx, storeSizeFn...)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("store manifest: %w", err)
	}
	logger.Tracef("finished uploaded dir with reference %v", manifestReference)

	return manifestReference, nil
}

type FileInfo struct {
	Path        string
	Name        string
	ContentType string
	Size        int64
	Reader      io.Reader
}

type dirReader interface {
	Next() (*FileInfo, error)
}

type tarReader struct {
	r      *tar.Reader
	logger logging.Logger
}

func (t *tarReader) Next() (*FileInfo, error) {
	for {
		fileHeader, err := t.r.Next()
		if err != nil {
			return nil, err
		}

		fileName := fileHeader.FileInfo().Name()
		contentType := mime.TypeByExtension(filepath.Ext(fileHeader.Name))
		fileSize := fileHeader.FileInfo().Size()
		filePath := filepath.Clean(fileHeader.Name)

		if filePath == "." {
			t.logger.Warning("skipping file upload empty path")
			continue
		}
		if runtime.GOOS == "windows" {
			// always use Unix path separator
			filePath = filepath.ToSlash(filePath)
		}
		// only store regular files
		if !fileHeader.FileInfo().Mode().IsRegular() {
			t.logger.Warningf("skipping file upload for %s as it is not a regular file", filePath)
			continue
		}

		return &FileInfo{
			Path:        filePath,
			Name:        fileName,
			ContentType: contentType,
			Size:        fileSize,
			Reader:      t.r,
		}, nil
	}
}

type multipartReader struct {
	r      *multipart.Reader
	logger logging.Logger
}

func (m *multipartReader) Next() (*FileInfo, error) {
	part, err := m.r.NextPart()
	if err != nil {
		return nil, err
	}

	fileName := part.FileName()
	if fileName == "" {
		fileName = part.FormName()
	}
	if fileName == "" {
		m.logger.Error("filename missing in multipart request")
		return nil, errors.New("filename missing")
	}

	contentType := part.Header.Get(contentTypeHeader)
	if contentType == "" {
		m.logger.Error("content-type missing in multipart request")
		return nil, errors.New("content-type missing")
	}

	contentLength := part.Header.Get("Content-Length")
	if contentLength == "" {
		m.logger.Error("content-length missing in multipart request")
		return nil, errors.New("content-length missing")
	}
	fileSize, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		m.logger.Errorf("Failed to parse content-length %s Err:%s",
			contentLength, err.Error())
		return nil, errors.New("invalid file size")
	}

	if filepath.Dir(fileName) != "." {
		m.logger.Errorf("Invalid filename for multipart request: %s", fileName)
		return nil, errors.New("multipart upload supports only single directory")
	}

	return &FileInfo{
		Path:        fileName,
		Name:        fileName,
		ContentType: contentType,
		Size:        fileSize,
		Reader:      part,
	}, nil
}

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
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
)

var errEmptyDir = errors.New("no files in root directory")

// dirUploadHandler uploads a directory supplied as a tar in an HTTP request
func (s *Service) dirUploadHandler(logger log.Logger, w http.ResponseWriter, r *http.Request, storer storage.Storer, waitFn func() error) {
	if r.Body == http.NoBody {
		logger.Error(nil, "request has no body")
		jsonhttp.BadRequest(w, errInvalidRequest)
		return
	}

	// The error is ignored because the header was already validated by the caller.
	mediaType, params, _ := mime.ParseMediaType(r.Header.Get(ContentTypeHeader))

	var dReader dirReader
	switch mediaType {
	case contentTypeTar:
		dReader = &tarReader{r: tar.NewReader(r.Body), logger: s.logger}
	case multiPartFormData:
		dReader = &multipartReader{r: multipart.NewReader(r.Body, params["boundary"])}
	default:
		logger.Error(nil, "invalid content-type for directory upload")
		jsonhttp.BadRequest(w, errInvalidContentType)
		return
	}
	defer r.Body.Close()

	tag, created, err := s.getOrCreateTag(r.Header.Get(SwarmTagHeader))
	if err != nil {
		logger.Debug("get or create tag failed", "error", err)
		logger.Error(nil, "get or create tag failed")
		switch {
		case errors.Is(err, tags.ErrNotFound):
			jsonhttp.NotFound(w, "tag not found")
		default:
			jsonhttp.InternalServerError(w, "cannot get or create tag")
		}
	}

	// Add the tag to the context
	ctx := sctx.SetTag(r.Context(), tag)

	reference, err := storeDir(
		ctx,
		requestEncrypt(r),
		dReader,
		s.logger,
		requestPipelineFn(storer, r),
		loadsave.New(storer, requestPipelineFactory(ctx, storer, r)),
		r.Header.Get(SwarmIndexDocumentHeader),
		r.Header.Get(SwarmErrorDocumentHeader),
		tag,
		created,
	)
	if err != nil {
		logger.Debug("store dir failed", "error", err)
		logger.Error(nil, "store dir failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		case errors.Is(err, errEmptyDir):
			jsonhttp.BadRequest(w, errEmptyDir)
		case errors.Is(err, tar.ErrHeader):
			jsonhttp.BadRequest(w, "invalid filename in tar archive")
		default:
			jsonhttp.InternalServerError(w, errDirectoryStore)
		}
		return
	}
	if created {
		_, err = tag.DoneSplit(reference)
		if err != nil {
			logger.Debug("done split failed", "error", err)
			logger.Error(nil, "done split failed")
			jsonhttp.InternalServerError(w, "done split failed")
			return
		}
	}

	if requestPin(r) {
		if err := s.pinning.CreatePin(r.Context(), reference, false); err != nil {
			logger.Debug("pin creation failed", "address", reference, "error", err)
			logger.Error(nil, "pin creation failed")
			jsonhttp.InternalServerError(w, "create pin failed")
			return
		}
	}

	if err = waitFn(); err != nil {
		logger.Debug("sync chunks failed", "error", err)
		logger.Error(nil, "sync chunks failed")
		jsonhttp.InternalServerError(w, "sync chunks failed")
		return
	}

	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	w.Header().Set(SwarmTagHeader, fmt.Sprint(tag.Uid))
	jsonhttp.Created(w, bzzUploadResponse{
		Reference: reference,
	})
}

// storeDir stores all files recursively contained in the directory given as a tar/multipart
// it returns the hash for the uploaded manifest corresponding to the uploaded dir
func storeDir(
	ctx context.Context,
	encrypt bool,
	reader dirReader,
	log log.Logger,
	p pipelineFunc,
	ls file.LoadSaver,
	indexFilename,
	errorFilename string,
	tag *tags.Tag,
	tagCreated bool,
) (swarm.Address, error) {
	logger := tracing.NewLoggerWithTraceID(ctx, log)
	loggerV1 := logger.V(1).Build()

	dirManifest, err := manifest.NewDefaultManifest(ls, encrypt)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	if indexFilename != "" && strings.ContainsRune(indexFilename, '/') {
		return swarm.ZeroAddress, errors.New("index document suffix must not include slash character")
	}

	filesAdded := 0

	// iterate through the files in the supplied tar
	for {
		fileInfo, err := reader.Next()
		if errors.Is(err, io.EOF) {
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

		fileReference, err := p(ctx, fileInfo.Reader)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("store dir file: %w", err)
		}
		loggerV1.Debug("bzz upload dir: file dir uploaded", "file_path", fileInfo.Path, "address", fileReference)

		fileMtdt := map[string]string{
			manifest.EntryMetadataContentTypeKey: fileInfo.ContentType,
			manifest.EntryMetadataFilenameKey:    fileInfo.Name,
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
		return swarm.ZeroAddress, errEmptyDir
	}

	// store website information
	if indexFilename != "" || errorFilename != "" {
		metadata := map[string]string{}
		if indexFilename != "" {
			metadata[manifest.WebsiteIndexDocumentSuffixKey] = indexFilename
		}
		if errorFilename != "" {
			metadata[manifest.WebsiteErrorDocumentPathKey] = errorFilename
		}
		rootManifestEntry := manifest.NewEntry(swarm.ZeroAddress, metadata)
		err = dirManifest.Add(ctx, manifest.RootPath, rootManifestEntry)
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
	loggerV1.Debug("bzz upload dir: uploaded dir finished", "address", manifestReference)

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
	logger log.Logger
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
			t.logger.Warning("bzz upload dir: skipping file upload as it is not a regular file", "file_path", filePath)
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

// multipart reader returns files added as a multipart form. We will ensure all the
// part headers are passed correctly
type multipartReader struct {
	r *multipart.Reader
}

func (m *multipartReader) Next() (*FileInfo, error) {
	part, err := m.r.NextPart()
	if err != nil {
		return nil, err
	}

	filePath := part.FileName()
	if filePath == "" {
		filePath = part.FormName()
	}
	if filePath == "" {
		return nil, errors.New("filepath missing")
	}

	fileName := filepath.Base(filePath)

	contentType := part.Header.Get(ContentTypeHeader)
	if contentType == "" {
		return nil, errors.New("content-type missing")
	}

	contentLength := part.Header.Get(ContentLengthHeader)
	if contentLength == "" {
		return nil, errors.New("content-length missing")
	}
	fileSize, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return nil, errors.New("invalid file size")
	}

	return &FileInfo{
		Path:        filePath,
		Name:        fileName,
		ContentType: contentType,
		Size:        fileSize,
		Reader:      part,
	}, nil
}

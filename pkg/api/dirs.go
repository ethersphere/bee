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

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"
)

var errEmptyDir = errors.New("no files in root directory")

// dirUploadHandler uploads a directory supplied as a tar in an HTTP request
func (s *Service) dirUploadHandler(
	ctx context.Context,
	logger log.Logger,
	span opentracing.Span,
	w http.ResponseWriter,
	r *http.Request,
	putter storer.PutterSession,
	contentTypeString string,
	encrypt bool,
	tag uint64,
	rLevel redundancy.Level,
	act bool,
	historyAddress swarm.Address,
) {
	if r.Body == http.NoBody {
		logger.Error(nil, "request has no body")
		jsonhttp.BadRequest(w, errInvalidRequest)
		return
	}

	// The error is ignored because the header was already validated by the caller.
	mediaType, params, _ := mime.ParseMediaType(contentTypeString)

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

	reference, err := storeDir(
		ctx,
		encrypt,
		dReader,
		logger,
		putter,
		s.storer.ChunkStore(),
		r.Header.Get(SwarmIndexDocumentHeader),
		r.Header.Get(SwarmErrorDocumentHeader),
		rLevel,
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
		ext.LogError(span, err, olog.String("action", "dir.store"))
		return
	}

	encryptedReference := reference
	if act {
		encryptedReference, err = s.actEncryptionHandler(r.Context(), w, putter, reference, historyAddress)
		if err != nil {
			logger.Debug("access control upload failed", "error", err)
			logger.Error(nil, "access control upload failed")
			switch {
			case errors.Is(err, accesscontrol.ErrNotFound):
				jsonhttp.NotFound(w, "act or history entry not found")
			case errors.Is(err, accesscontrol.ErrInvalidPublicKey) || errors.Is(err, accesscontrol.ErrSecretKeyInfinity):
				jsonhttp.BadRequest(w, "invalid public key")
			case errors.Is(err, accesscontrol.ErrUnexpectedType):
				jsonhttp.BadRequest(w, "failed to create history")
			default:
				jsonhttp.InternalServerError(w, errActUpload)
			}
			return
		}
	}

	err = putter.Done(reference)
	if err != nil {
		logger.Debug("store dir failed", "error", err)
		logger.Error(nil, "store dir failed")
		jsonhttp.InternalServerError(w, errDirectoryStore)
		ext.LogError(span, err, olog.String("action", "putter.Done"))
		return
	}

	if tag != 0 {
		w.Header().Set(SwarmTagHeader, fmt.Sprint(tag))
		span.LogFields(olog.Bool("success", true))
	}
	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, bzzUploadResponse{
		Reference: encryptedReference,
	})
}

// storeDir stores all files recursively contained in the directory given as a tar/multipart
// it returns the hash for the uploaded manifest corresponding to the uploaded dir
func storeDir(
	ctx context.Context,
	encrypt bool,
	reader dirReader,
	log log.Logger,
	putter storage.Putter,
	getter storage.Getter,
	indexFilename,
	errorFilename string,
	rLevel redundancy.Level,
) (swarm.Address, error) {

	logger := tracing.NewLoggerWithTraceID(ctx, log)
	loggerV1 := logger.V(1).Build()

	p := requestPipelineFn(putter, encrypt, rLevel)
	ls := loadsave.New(getter, putter, requestPipelineFactory(ctx, putter, encrypt, rLevel))

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
			return swarm.ZeroAddress, fmt.Errorf("read dir stream: %w", err)
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

	// save manifest
	manifestReference, err := dirManifest.Store(ctx)
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

	fileName := filepath.Base(filePath)

	contentType := part.Header.Get(ContentTypeHeader)

	contentLength := part.Header.Get(ContentLengthHeader)

	fileSize, _ := strconv.ParseInt(contentLength, 10, 64)

	return &FileInfo{
		Path:        filePath,
		Name:        fileName,
		ContentType: contentType,
		Size:        fileSize,
		Reader:      part,
	}, nil
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	contentTypeHeader = "Content-Type"
	contentTypeTar    = "application/x-tar"
)

type toEncryptContextKey struct{}

// dirUploadHandler uploads a directory supplied as a tar in an HTTP request
func (s *server) dirUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx, err := validateRequest(r)
	if err != nil {
		s.Logger.Errorf("dir upload, validate request")
		s.Logger.Debugf("dir upload, validate request err: %v", err)
		jsonhttp.BadRequest(w, "could not validate request")
		return
	}

	tag, created, err := s.getOrCreateTag(r.Header.Get(SwarmTagUidHeader))
	if err != nil {
		s.Logger.Debugf("dir upload: get or create tag: %v", err)
		s.Logger.Error("dir upload: get or create tag")
		jsonhttp.InternalServerError(w, "cannot get or create tag")
		return
	}

	// Add the tag to the context
	ctx = sctx.SetTag(ctx, tag)

	reference, err := storeDir(ctx, r.Body, s.Storer, requestModePut(r), s.Logger)
	if err != nil {
		s.Logger.Debugf("dir upload, store dir err: %v", err)
		s.Logger.Errorf("dir upload, store dir")
		jsonhttp.InternalServerError(w, "could not store dir")
		return
	}
	if created {
		tag.DoneSplit(reference)
	}
	w.Header().Set(SwarmTagUidHeader, fmt.Sprint(tag.Uid))
	jsonhttp.OK(w, fileUploadResponse{
		Reference: reference,
	})
}

// validateRequest validates an HTTP request for a directory to be uploaded
// it returns a context based on the given request
func validateRequest(r *http.Request) (context.Context, error) {
	ctx := r.Context()
	if r.Body == http.NoBody {
		return nil, errors.New("request has no body")
	}
	contentType := r.Header.Get(contentTypeHeader)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	if mediaType != contentTypeTar {
		return nil, errors.New("content-type not set to tar")
	}
	toEncrypt := strings.ToLower(r.Header.Get(EncryptHeader)) == "true"
	return context.WithValue(ctx, toEncryptContextKey{}, toEncrypt), nil
}

// storeDir stores all files recursively contained in the directory given as a tar
// it returns the hash for the uploaded manifest corresponding to the uploaded dir
func storeDir(ctx context.Context, reader io.ReadCloser, s storage.Storer, mode storage.ModePut, logger logging.Logger) (swarm.Address, error) {
	v := ctx.Value(toEncryptContextKey{})
	toEncrypt, _ := v.(bool) // default is false

	dirManifest, err := manifest.NewDefaultManifest(ctx, toEncrypt, s)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// set up HTTP body reader
	tarReader := tar.NewReader(reader)
	defer reader.Close()

	filesAdded := 0

	// iterate through the files in the supplied tar
	for {
		fileHeader, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("read tar stream: %w", err)
		}

		filePath := fileHeader.Name

		// only store regular files
		if !fileHeader.FileInfo().Mode().IsRegular() {
			logger.Warningf("skipping file upload for %s as it is not a regular file", filePath)
			continue
		}

		fileName := fileHeader.FileInfo().Name()
		contentType := mime.TypeByExtension(filepath.Ext(fileHeader.Name))

		// upload file
		fileInfo := &fileUploadInfo{
			name:        fileName,
			size:        fileHeader.FileInfo().Size(),
			contentType: contentType,
			reader:      tarReader,
		}
		fileReference, err := storeFile(ctx, fileInfo, s, mode)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("store dir file: %w", err)
		}
		logger.Tracef("uploaded dir file %v with reference %v", filePath, fileReference)

		// add file entry to dir manifest
		err = dirManifest.Add(filePath, manifest.NewEntry(fileReference))
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("add to manifest: %w", err)
		}

		filesAdded++
	}

	// check if files were uploaded through the manifest
	if filesAdded == 0 {
		return swarm.ZeroAddress, fmt.Errorf("no files in tar")
	}

	// save manifest
	manifestBytesReference, err := dirManifest.Store(mode)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("store manifest: %w", err)
	}

	// store the manifest metadata and get its reference
	m := entry.NewMetadata(manifestBytesReference.String())
	m.MimeType = dirManifest.Type()
	metadataBytes, err := json.Marshal(m)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("metadata marshal: %w", err)
	}

	sp := splitter.NewSimpleSplitter(s, mode)
	mr, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split metadata: %w", err)
	}

	// now join both references (fr, mr) to create an entry and store it
	e := entry.New(manifestBytesReference, mr)
	fileEntryBytes, err := e.MarshalBinary()
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("entry marshal: %w", err)
	}

	sp = splitter.NewSimpleSplitter(s, mode)
	manifestFileReference, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)), toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split entry: %w", err)
	}

	return manifestFileReference, nil
}

// storeFile uploads the given file and returns its reference
// this function was extracted from `fileUploadHandler` and should eventually replace its current code
func storeFile(ctx context.Context, fileInfo *fileUploadInfo, s storage.Storer, mode storage.ModePut) (swarm.Address, error) {
	v := ctx.Value(toEncryptContextKey{})
	toEncrypt, _ := v.(bool) // default is false

	// first store the file and get its reference
	sp := splitter.NewSimpleSplitter(s, mode)
	fr, err := file.SplitWriteAll(ctx, sp, fileInfo.reader, fileInfo.size, toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split file: %w", err)
	}

	// if filename is still empty, use the file hash as the filename
	if fileInfo.name == "" {
		fileInfo.name = fr.String()
	}

	// then store the metadata and get its reference
	m := entry.NewMetadata(fileInfo.name)
	m.MimeType = fileInfo.contentType
	metadataBytes, err := json.Marshal(m)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("metadata marshal: %w", err)
	}

	sp = splitter.NewSimpleSplitter(s, mode)
	mr, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split metadata: %w", err)
	}

	// now join both references (mr, fr) to create an entry and store it
	e := entry.New(fr, mr)
	fileEntryBytes, err := e.MarshalBinary()
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("entry marshal: %w", err)
	}
	sp = splitter.NewSimpleSplitter(s, mode)
	reference, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)), toEncrypt)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("split entry: %w", err)
	}

	return reference, nil
}

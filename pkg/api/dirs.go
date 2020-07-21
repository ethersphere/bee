// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest/jsonmanifest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// dirUploadResponse is returned when an HTTP request to upload a directory as a tar is successful
type dirUploadResponse struct {
	Reference swarm.Address `json:"reference"` // manifest hash for the uploaded dir
}

// dirUploadInfo contains the data for a directory to be uploaded
type dirUploadInfo struct {
	toEncrypt bool
	reader    io.ReadCloser
}

// dirUploadHandler uploads a directory supplied as a tar in an HTTP request
func (s *server) dirUploadHandler(w http.ResponseWriter, r *http.Request) {
	dirInfo, err := getDirHTTPInfo(r)
	if err != nil {
		s.Logger.Errorf("dir upload, get dir info: %v", err)
		jsonhttp.BadRequest(w, "could not extract dir info from request")
		return
	}

	reference, err := storeDir(r.Context(), dirInfo, s.Storer, s.Logger)
	if err != nil {
		s.Logger.Errorf("dir upload, store dir: %v", err)
		jsonhttp.InternalServerError(w, "could not store dir")
		return
	}

	jsonhttp.OK(w, dirUploadResponse{
		Reference: reference,
	})
}

// getDirHTTPInfo extracts data for a directory to be uploaded from an HTTP request
func getDirHTTPInfo(r *http.Request) (*dirUploadInfo, error) {
	if r.Body == http.NoBody {
		return &dirUploadInfo{}, errors.New("request has no body")
	}
	toEncrypt := strings.ToLower(r.Header.Get(encryptHeader)) == "true"
	return &dirUploadInfo{
		reader:    r.Body,
		toEncrypt: toEncrypt,
	}, nil
}

// storeDir stores all files recursively contained in the directory given as a tar
// it returns the hash for the uploaded manifest corresponding to the uploaded dir
func storeDir(ctx context.Context, dirInfo *dirUploadInfo, s storage.Storer, logger logging.Logger) (swarm.Address, error) {
	dirManifest := jsonmanifest.NewManifest()

	// set up HTTP body reader
	tarReader := tar.NewReader(dirInfo.reader)
	defer dirInfo.reader.Close()

	// iterate through the files in the supplied tar
	for {
		fileHeader, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("read tar stream error: %v", err)
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
		fileInfo := &FileUploadInfo{
			name:        fileName,
			size:        fileHeader.FileInfo().Size(),
			contentType: contentType,
			toEncrypt:   dirInfo.toEncrypt,
			reader:      tarReader,
		}
		fileReference, err := storeFile(ctx, fileInfo, s)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("store dir file error: %v", err)
		}
		logger.Tracef("uploaded dir file %v with reference %v", filePath, fileReference)

		// create manifest entry for uploaded file
		headers := http.Header{}
		headers.Set("Content-Type", contentType)
		entry := &jsonmanifest.JSONEntry{
			Reference: fileReference,
			Name:      fileName,
			Headers:   headers,
		}

		// add entry to dir manifest
		dirManifest.Add(filePath, entry)
	}

	// upload manifest
	// first, serialize into byte array
	b, err := dirManifest.Serialize()
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest serialize error: %v", err)
	}

	// set up reader for manifest file upload
	r := bytes.NewReader(b)

	// then, upload manifest
	manifestFileInfo := &FileUploadInfo{
		name:        "manifest.json",
		size:        r.Size(),
		contentType: ManifestContentType,
		toEncrypt:   dirInfo.toEncrypt,
		reader:      r,
	}
	manifestReference, err := storeFile(ctx, manifestFileInfo, s)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("store manifest error: %v", err)
	}

	return manifestReference, nil
}

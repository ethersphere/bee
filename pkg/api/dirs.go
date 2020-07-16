// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type dirUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

// dirUploadHandler uploads a directory supplied as a tar in an HTTP Request
func (s *server) dirUploadHandler(w http.ResponseWriter, r *http.Request) {
	dirInfo, err := GetDirHTTPInfo(r)
	if err != nil {
		s.Logger.Errorf("dir upload, get dir info: %v", err)
		jsonhttp.BadRequest(w, "could not extract dir info from request")
		return
	}

	reference, err := StoreDir(r.Context(), dirInfo, s.Storer, s.Logger)
	if err != nil {
		s.Logger.Errorf("dir upload, store dir: %v", err)
		jsonhttp.InternalServerError(w, "could not store dir")
		return
	}

	jsonhttp.OK(w, dirUploadResponse{
		Reference: reference,
	})
}

// DirUploadInfo contains the data for a dir to be uploaded
type DirUploadInfo struct {
	DefaultPath string
	DirReader   io.ReadCloser
	ToEncrypt   bool
}

// GetDirHTTPInfo extracts dir info for upload from HTTP request
func GetDirHTTPInfo(r *http.Request) (*DirUploadInfo, error) {
	defaultPath := r.URL.Query().Get("defaultpath")
	toEncrypt := strings.ToLower(r.Header.Get(encryptHeader)) == "true"
	return &DirUploadInfo{
		DefaultPath: defaultPath,
		DirReader:   r.Body,
		ToEncrypt:   toEncrypt,
	}, nil
}

// StoreDir stores all files contained in the given directory as a tar and returns its reference
func StoreDir(ctx context.Context, dirInfo *DirUploadInfo, s storage.Storer, logger logging.Logger) (swarm.Address, error) {
	var contentKey swarm.Address // how is this determined?
	// manifestPath := GetURI(r.Context()).Path // ??

	bodyReader := dirInfo.DirReader
	tr := tar.NewReader(bodyReader)
	defer bodyReader.Close()

	var defaultPathFound bool
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("read tar stream error: %v", err)
		}

		// only store regular files
		if !hdr.FileInfo().Mode().IsRegular() { // ??
			continue
		}

		// manifestPath := path.Join(manifestPath, hdr.Name)
		// apparently, `h.Xattrs[key] = value` == `h.PAXRecords["SCHILY.xattr."+key] = value`
		contentType := hdr.PAXRecords["SCHILY.xattr."+"user.swarm.content-type"]
		if contentType == "" {
			contentType = mime.TypeByExtension(filepath.Ext(hdr.Name))
		}

		// add the entry under the path from the request
		/*
			entry := &ManifestEntry{
				Path:        manifestPath,
				ContentType: contentType,
				Mode:        hdr.Mode,
				Size:        hdr.Size,
				ModTime:     hdr.ModTime,
			}
			contentKey, err = mw.AddEntry(ctx, tr, entry)
			if err != nil {
				return nil, fmt.Errorf("error adding manifest entry from tar stream: %s", err)
			}*/

		if hdr.Name == dirInfo.DefaultPath {
			/*contentType := hdr.Xattrs["user.swarm.content-type"]
			if contentType == "" {
				contentType = mime.TypeByExtension(filepath.Ext(hdr.Name))
			}

			entry := &ManifestEntry{
				Hash:        contentKey.Hex(),
				Path:        "", // default entry
				ContentType: contentType,
				Mode:        hdr.Mode,
				Size:        hdr.Size,
				ModTime:     hdr.ModTime,
			}
			conctx context.Context,return nil, fmt.Errorf("error adding default manifest entry from tar stream: %s", err)
			}*/

			defaultPathFound = true
		}

		fileInfo := &FileUploadInfo{
			FileName:    hdr.Name,
			FileSize:    hdr.Size,
			ContentType: contentType,
			ToEncrypt:   dirInfo.ToEncrypt,
			Reader:      tr,
		}
		fileReference, err := StoreFile(ctx, fileInfo, s)

		logger.Infof("fileReference: %v", fileReference)

		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("store dir file error: %v", err)
		}

		_ = fileReference // what do we do with each file ref?
	}

	if dirInfo.DefaultPath != "" && !defaultPathFound {
		// TODO: should we still return the content key _plus_ the error?
		return swarm.ZeroAddress, fmt.Errorf("default path error: %s not found", dirInfo.DefaultPath)
	}

	return contentKey, nil
}

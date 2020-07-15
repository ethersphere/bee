// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"archive/tar"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/upload"
)

type dirUploadResponse struct {
	Reference swarm.Address `json:"reference"`
}

// dirUploadHandler uploads a directory
// for now, adapted from old swarm tar upload code
func (s *server) dirUploadHandler(w http.ResponseWriter, r *http.Request) {
	dirInfo, err := upload.GetDirHTTPInfo(r)
	if err != nil {
		s.Logger.Debugf("dir upload: get dir info, request %v: %v", *r, err)
		s.Logger.Errorf("dir upload: get dir info, request %v", *r)
		jsonhttp.BadRequest(w, "could not extract dir info from request")
		return
	}

	var contentKey swarm.Address
	// TODO
	// manifestPath := ?

	bodyReader := dirInfo.DirReader
	tr := tar.NewReader(bodyReader)
	defer bodyReader.Close()

	// TODO: add Bad Request response for non-tar files?

	var defaultPathFound bool
	// upload (add entry?) each file read in tar
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			jsonhttp.BadRequest(w, fmt.Sprintf("error reading tar stream: %s", err.Error()))
			return
		}

		// only store regular files
		// TODO: look into this
		if !hdr.FileInfo().Mode().IsRegular() {
			continue
		}

		// add the entry under the path from the request
		// TODO: skip for now
		// manifestPath := path.Join(manifestPath, hdr.Name)
		contentType := hdr.Xattrs["user.swarm.content-type"]
		if contentType == "" {
			contentType = mime.TypeByExtension(filepath.Ext(hdr.Name))
		}

		// TODO: skip for now
		/*
			entry := &ManifestEntry{
				Path:        manifestPath,
				ContentType: contentType,
				Mode:        hdr.Mode,
				Size:        hdr.Size,
				ModTime:     hdr.ModTime,
			}
			contentKey, err = mw.AddEntry(ctx, tr, entry)*/
		if err != nil {
			//return nil, fmt.Errorf("error adding manifest entry from tar stream: %s", err)
		}
		if hdr.Name == dirInfo.DefaultPath {
			contentType := hdr.Xattrs["user.swarm.content-type"]
			if contentType == "" {
				contentType = mime.TypeByExtension(filepath.Ext(hdr.Name))
			}

			// TODO: skip for now
			/*entry := &ManifestEntry{
				Hash:        contentKey.Hex(),
				Path:        "", // default entry
				ContentType: contentType,
				Mode:        hdr.Mode,
				Size:        hdr.Size,
				ModTime:     hdr.ModTime,
			}
			contentKey, err = mw.AddEntry(ctx, nil, entry)*/
			if err != nil {
				//return nil, fmt.Errorf("error adding default manifest entry from tar stream: %s", err)
			}
			defaultPathFound = true
		}
	}
	if dirInfo.DefaultPath != "" && !defaultPathFound {
		// TODO: should we still return the content key _plus_ the error?
		jsonhttp.BadRequest(w, fmt.Sprintf("default path %s not found", dirInfo.DefaultPath))
	}

	jsonhttp.OK(w, dirUploadResponse{
		Reference: contentKey,
	})
}

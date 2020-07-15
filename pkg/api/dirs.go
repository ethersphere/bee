// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

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

	reference, err := upload.StoreTar(dirInfo)
	if err != nil {
		s.Logger.Debugf("dir upload: store dir, request %v: %v", *r, err)
		s.Logger.Errorf("dir upload: store dir, request %v", *r)
		jsonhttp.InternalServerError(w, "could store dir")
		return
	}

	jsonhttp.OK(w, dirUploadResponse{
		Reference: reference,
	})
}

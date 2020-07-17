// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *server) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	toEncrypt := strings.ToLower(r.Header.Get(encryptHeader)) == "true"
	sp := splitter.NewSimpleSplitter(s.Storer)
	address, err := file.SplitWriteAll(ctx, sp, r.Body, r.ContentLength, toEncrypt)
	if err != nil {
		s.Logger.Debugf("bytes upload: %v", err)
		jsonhttp.InternalServerError(w, nil)
		return
	}
	jsonhttp.OK(w, bytesPostResponse{
		Reference: address,
	})
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *server) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("bytes: parse address %s: %v", addressHex, err)
		s.Logger.Error("bytes: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	additionalHeaders := http.Header{
		"Content-Type": {"application/octet-stream"},
	}

	s.downloadHandler(w, r, address, additionalHeaders)
}

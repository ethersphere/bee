// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
)

type rawPostResponse struct {
	Hash swarm.Address `json:"hash"`
}

func (s *server) rawUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	s.Logger.Errorf("Content length %d", r.ContentLength)
	sp := splitter.NewSimpleSplitter(s.Storer)
	address, err := sp.Split(ctx, r.Body, r.ContentLength)
	if err != nil {
		s.Logger.Debugf("raw: split error %s: %v", address, err)
		s.Logger.Error("raw: split error")
		jsonhttp.BadRequest(w, "split error")
		return
	}
	jsonhttp.OK(w, rawPostResponse{Hash: address})
}

func (s *server) rawGetHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("raw: parse address %s: %v", addressHex, err)
		s.Logger.Error("raw: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	j := joiner.NewSimpleJoiner(s.Storer)

	dataSize, err := j.Size(ctx, address)
	if err != nil {
		s.Logger.Debugf("raw: invalid root chunk %s: %v", address, err)
		s.Logger.Error("raw: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	err = file.JoinReadAll(j, address, w)
	if err != nil {
		s.Logger.Errorf("raw: data write %s: %v", address, err)
		s.Logger.Error("raw: data input error")
		jsonhttp.BadRequest(w, "failed to retrieve data")
		return
	}
}

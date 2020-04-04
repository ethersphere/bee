// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

func (s *server) chunkUploadHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: parse chunk address %s: %v", addr, err)
		s.Logger.Error("bzz-chunk: error uploading chunk")
		jsonhttp.BadRequest(w, "invalid chunk address")
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: read chunk data error: %v, addr %s", err, address)
		s.Logger.Error("bzz-chunk: read chunk data error")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return

	}

	err = s.Storer.Put(ctx, address, data)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: chunk write error: %v, addr %s", err, address)
		s.Logger.Error("bzz-chunk: chunk write error")
		jsonhttp.BadRequest(w, "chunk write error")
		return
	}

	jsonhttp.OK(w, nil)
}

func (s *server) chunkGetHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: parse chunk address %s: %v", addr, err)
		s.Logger.Error("bzz-chunk: parse chunk address error")
		jsonhttp.BadRequest(w, "invalid chunk address")
		return
	}

	data, err := s.Storer.Get(ctx, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Trace("bzz-chunk: chunk not found. addr %s", address)
			jsonhttp.NotFound(w, "chunk not found")
			return

		}
		s.Logger.Debugf("bzz-chunk: chunk read error: %v ,addr %s", err, address)
		s.Logger.Error("bzz-chunk: chunk read error")
		jsonhttp.InternalServerError(w, "chunk read error")
		return
	}
	w.Header().Set("Content-Type", "binary/octet-stream")
	_, _ = io.Copy(w, bytes.NewReader(data))
}

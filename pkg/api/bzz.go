// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
)

type rawPostResponse struct {
	Hash swarm.Address `json:"hash"`
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
	outReader, dataSize, err := j.Join(ctx, address)
	if err != nil {
		s.Logger.Debugf("raw: join on address %s: %v", address, err)
		s.Logger.Error("raw: join on address error")
		jsonhttp.BadRequest(w, "failed to locate data")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	for {
		_, err = io.Copy(w, outReader)
		if err != io.EOF {
			s.Logger.Errorf("raw: data write %s: %v", address, err)
			s.Logger.Error("raw: data input error")
			jsonhttp.BadRequest(w, "failed to retrieve data")
			return
		}
	}
}

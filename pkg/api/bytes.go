// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *server) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	toEncrypt := strings.ToLower(r.Header.Get(EncryptHeader)) == "true"
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
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("bytes: parse address %s: %v", addressHex, err)
		s.Logger.Error("bytes: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	toDecrypt := len(address.Bytes()) == (swarm.HashSize + encryption.KeyLength)
	j := joiner.NewSimpleJoiner(s.Storer)
	dataSize, err := j.Size(ctx, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Debugf("bytes: not found %s: %v", address, err)
			s.Logger.Error("bytes: not found")
			jsonhttp.NotFound(w, "not found")
			return
		}
		s.Logger.Debugf("bytes: invalid root chunk %s: %v", address, err)
		s.Logger.Error("bytes: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	targets := r.URL.Query().Get("targets")
	r = r.WithContext(context.WithValue(r.Context(), "targets", targets))
	ctx = r.Context()

	outBuffer := bytes.NewBuffer(nil)
	c, err := file.JoinReadAll(ctx, j, address, outBuffer, toDecrypt)
	if err != nil && c == 0 {
		s.Logger.Debugf("bytes download: data join %s: %v", address, err)
		s.Logger.Errorf("bytes download: data join %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf("%q", address))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	if _, err = io.Copy(w, outBuffer); err != nil {
		s.Logger.Debugf("bytes download: data read %s: %v", address, err)
		s.Logger.Errorf("bytes download: data read %s", address)
	}
}

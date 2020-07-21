// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
<<<<<<< HEAD
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
=======
>>>>>>> master
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

	targets := r.URL.Query().Get("targets")
	r = r.WithContext(context.WithValue(r.Context(), targetsContextKey{}, targets))
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
	w.Header().Set("Targets", targets)
	if _, err = io.Copy(w, outBuffer); err != nil {
		s.Logger.Debugf("bytes download: data read %s: %v", address, err)
		s.Logger.Errorf("bytes download: data read %s", address)
	}
	s.downloadHandler(w, r, address, additionalHeaders)
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/binary"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *Service) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_bytes_by_address").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.IsZero() {
		address = v
	}

	additionalHeaders := http.Header{
		ContentTypeHeader: {"application/octet-stream"},
	}

	s.downloadHandler(logger, w, r, address, additionalHeaders, true, false, nil)
}

func (s *Service) bytesHeadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("head_bytes_by_address").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.IsZero() {
		address = v
	}

	getter := s.storer.Download(true)
	ch, err := getter.Get(r.Context(), address)
	if err != nil {
		logger.Debug("get root chunk failed", "chunk_address", address, "error", err)
		logger.Error(nil, "get root chunk failed")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Add(AccessControlExposeHeaders, "Accept-Ranges, Content-Encoding")
	w.Header().Add(ContentTypeHeader, "application/octet-stream")
	var span int64

	if cac.Valid(ch) {
		span = int64(binary.LittleEndian.Uint64(ch.Data()[:swarm.SpanSize]))
	} else {
		span = int64(len(ch.Data()))
	}
	w.Header().Set(ContentLengthHeader, strconv.FormatInt(span, 10))
	w.WriteHeader(http.StatusOK) // HEAD requests do not write a body
}

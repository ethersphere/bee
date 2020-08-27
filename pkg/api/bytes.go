// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *server) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	tag, created, err := s.getOrCreateTag(r.Header.Get(SwarmTagUidHeader))
	if err != nil {
		s.Logger.Debugf("bytes upload: get or create tag: %v", err)
		s.Logger.Error("bytes upload: get or create tag")
		jsonhttp.InternalServerError(w, "cannot get or create tag")
		return
	}

	// Add the tag to the context
	ctx := sctx.SetTag(r.Context(), tag)

	pipe := pipeline.NewPipelineBuilder(ctx, s.Storer, requestModePut(r), requestEncrypt(r))
	address, err := pipeline.FeedPipeline(ctx, pipe, r.Body, r.ContentLength)
	if err != nil {
		s.Logger.Debugf("bytes upload: split write all: %v", err)
		s.Logger.Error("bytes upload: split write all")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	if created {
		tag.DoneSplit(address)
	}
	w.Header().Set(SwarmTagUidHeader, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", SwarmTagUidHeader)
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

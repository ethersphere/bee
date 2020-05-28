// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ethersphere/bee/pkg/tags"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

// Presence of this header means that it needs to be tagged
const TagHeaderName = "x-swarm-tag"

// Presence of this header in the HTTP request indicates the chunk needs to be pinned.
const PinHeaderName = "x-swarm-pin"

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

	// create the Tag for the chunk
	tagName := r.Header.Get(TagHeaderName)
	if tagName == "" {
		tagName = fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
	}
	tag, err := s.Tags.Create(tagName, 1, false)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: tag creation error: %v, addr %s", err, address)
		s.Logger.Error("bzz-chunk: tag creation error")
		jsonhttp.InternalServerError(w, "cannot create tag")
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: read chunk data error: %v, addr %s", err, address)
		s.Logger.Error("bzz-chunk: read chunk data error")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return

	}

	seen, err := s.Storer.Put(ctx, storage.ModePutUpload, swarm.NewChunk(address, data))
	if err != nil {
		s.Logger.Debugf("bzz-chunk: chunk write error: %v, addr %s", err, address)
		s.Logger.Error("bzz-chunk: chunk write error")
		jsonhttp.BadRequest(w, "chunk write error")
		return
	} else if len(seen) > 0 && seen[0] {
		tag.Inc(tags.StateSeen)
	}

	// Indicate that the chunk is stored
	tag.Inc(tags.StateStored)

	// Check if this chunk needs to pinned and pin it
	pinHeaderValues := r.Header.Get(PinHeaderName)
	if pinHeaderValues != "" && strings.ToLower(pinHeaderValues) == "true" {
		err = s.Storer.Set(ctx, storage.ModeSetPin, address)
		if err != nil {
			s.Logger.Debugf("bzz-chunk: chunk pinning error: %v, addr %s", err, address)
			s.Logger.Error("bzz-chunk: chunk pinning error")
			jsonhttp.InternalServerError(w, "cannot pin chunk")
			return
		}
	}

	tag.Address = address
	w.Header().Set(TagHeaderName, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", TagHeaderName)

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

	chunk, err := s.Storer.Get(ctx, storage.ModeGetRequest, address)
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
	_, _ = io.Copy(w, bytes.NewReader(chunk.Data()))
}

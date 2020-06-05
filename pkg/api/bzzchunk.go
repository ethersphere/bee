// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/mux"
)

// Presence of this header means that it needs to be tagged using the uid
const TagHeaderUid = "x-swarm-tag-uid"

// Presence of this header in the HTTP request indicates the chunk needs to be pinned.
const PinHeaderName = "x-swarm-pin"

func (s *server) chunkUploadHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: parse chunk address %s: %v", addr, err)
		s.Logger.Error("bzz-chunk: parse chunk address")
		jsonhttp.BadRequest(w, "invalid chunk address")
		return
	}

	// if tag header is not there create a new one
	var tag *tags.Tag
	tagUidStr := r.Header.Get(TagHeaderUid)
	if tagUidStr == "" {
		tagName := fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
		tag, err = s.Tags.Create(tagName, 0, false)
		if err != nil {
			s.Logger.Debugf("bzz-chunk: tag creation error: %v, addr %s", err, address)
			s.Logger.Error("bzz-chunk: tag creation error")
			jsonhttp.InternalServerError(w, "cannot create tag")
			return
		}
	} else {
		// if the tag uid header is present, then use the tag sent
		tagUid, err := strconv.ParseUint(tagUidStr, 10, 32)
		if err != nil {
			s.Logger.Debugf("bzz-chunk: parse taguid %s: %v", tagUidStr, err)
			s.Logger.Error("bzz-chunk: parse taguid")
			jsonhttp.BadRequest(w, "invalid taguid")
			return
		}

		tag, err = s.Tags.Get(uint32(tagUid))
		if err != nil {
			s.Logger.Debugf("bzz-chunk: tag get error: %v, addr %s", err, address)
			s.Logger.Error("bzz-chunk: tag get error")
			jsonhttp.InternalServerError(w, "cannot create tag")
			return
		}
	}

	// Increment the total tags here since we dont have a splitter
	// for the file upload, it will done in the early stage itself in bulk
	tag.Inc(tags.TotalChunks)

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: read chunk data error: %v, addr %s", err, address)
		s.Logger.Error("bzz-chunk: read chunk data error")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	span := len(data)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(span))
	data = append(b, data...)

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
	w.Header().Set(TagHeaderUid, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", TagHeaderUid)
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
	_, _ = io.Copy(w, bytes.NewReader(chunk.Data()[8:]))
}

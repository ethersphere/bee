// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
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
const TagHeaderUid = "swarm-tag-uid"

// Presence of this header in the HTTP request indicates the chunk needs to be pinned.
const PinHeaderName = "swarm-pin"

func (s *server) chunkUploadHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("chunk upload: parse chunk address %s: %v", addr, err)
		s.Logger.Error("chunk upload: parse chunk address")
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
			s.Logger.Debugf("chunk upload: tag creation error: %v, addr %s", err, address)
			s.Logger.Error("chunk upload: tag creation error")
			jsonhttp.InternalServerError(w, "cannot create tag")
			return
		}
	} else {
		// if the tag uid header is present, then use the tag sent
		tagUid, err := strconv.ParseUint(tagUidStr, 10, 32)
		if err != nil {
			s.Logger.Debugf("chunk upload: parse taguid %s: %v", tagUidStr, err)
			s.Logger.Error("chunk upload: parse taguid")
			jsonhttp.BadRequest(w, "invalid taguid")
			return
		}

		tag, err = s.Tags.Get(uint32(tagUid))
		if err != nil {
			s.Logger.Debugf("chunk upload: tag get error: %v, addr %s", err, address)
			s.Logger.Error("chunk upload: tag get error")
			jsonhttp.InternalServerError(w, "cannot create tag")
			return
		}
	}

	// Increment the StateSplit here since we dont have a splitter for the file upload
	tag.Inc(tags.StateSplit)

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.Logger.Debugf("chunk upload: read chunk data error: %v, addr %s", err, address)
		s.Logger.Error("chunk upload: read chunk data error")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return

	}

	seen, err := s.Storer.Put(ctx, storage.ModePutUpload, swarm.NewChunk(address, data))
	if err != nil {
		s.Logger.Debugf("chunk upload: chunk write error: %v, addr %s", err, address)
		s.Logger.Error("chunk upload: chunk write error")
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
			s.Logger.Debugf("chunk upload: chunk pinning error: %v, addr %s", err, address)
			s.Logger.Error("chunk upload: chunk pinning error")
			jsonhttp.InternalServerError(w, "cannot pin chunk")
			return
		}
	}

	tag.DoneSplit(address)

	w.Header().Set(TagHeaderUid, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", TagHeaderUid)
	jsonhttp.OK(w, nil)
}

func (s *server) chunkGetHandler(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("chunk: parse chunk address %s: %v", addr, err)
		s.Logger.Error("chunk: parse chunk address error")
		jsonhttp.BadRequest(w, "invalid chunk address")
		return
	}

	chunk, err := s.Storer.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Trace("chunk: chunk not found. addr %s", address)
			jsonhttp.NotFound(w, "chunk not found")
			return

		}
		s.Logger.Debugf("chunk: chunk read error: %v ,addr %s", err, address)
		s.Logger.Error("chunk: chunk read error")
		jsonhttp.InternalServerError(w, "chunk read error")
		return
	}
	w.Header().Set("Content-Type", "binary/octet-stream")
	_, _ = io.Copy(w, bytes.NewReader(chunk.Data()))
}

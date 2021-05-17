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
	"strings"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/netstore"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/mux"
)

type chunkAddressResponse struct {
	Reference swarm.Address `json:"reference"`
}

func (s *server) chunkUploadHandler(w http.ResponseWriter, r *http.Request) {
	var (
		tag *tags.Tag
		ctx = r.Context()
		err error
	)

	if h := r.Header.Get(SwarmTagHeader); h != "" {
		tag, err = s.getTag(h)
		if err != nil {
			s.logger.Debugf("chunk upload: get tag: %v", err)
			s.logger.Error("chunk upload: get tag")
			jsonhttp.BadRequest(w, "cannot get tag")
			return

		}

		// add the tag to the context if it exists
		ctx = sctx.SetTag(r.Context(), tag)

		// increment the StateSplit here since we dont have a splitter for the file upload
		err = tag.Inc(tags.StateSplit)
		if err != nil {
			s.logger.Debugf("chunk upload: increment tag: %v", err)
			s.logger.Error("chunk upload: increment tag")
			jsonhttp.InternalServerError(w, "increment tag")
			return
		}
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debugf("chunk upload: read chunk data error: %v", err)
		s.logger.Error("chunk upload: read chunk data error")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if len(data) < swarm.SpanSize {
		s.logger.Debug("chunk upload: not enough data")
		s.logger.Error("chunk upload: data length")
		jsonhttp.BadRequest(w, "data length")
		return
	}

	chunk, err := cac.NewWithDataSpan(data)
	if err != nil {
		s.logger.Debugf("chunk upload: create chunk error: %v", err)
		s.logger.Error("chunk upload: create chunk error")
		jsonhttp.InternalServerError(w, "create chunk error")
		return
	}

	batch, err := requestPostageBatchId(r)
	if err != nil {
		s.logger.Debugf("chunk upload: postage batch id: %v", err)
		s.logger.Error("chunk upload: postage batch id")
		jsonhttp.BadRequest(w, "invalid postage batch id")
		return
	}

	putter, err := newStamperPutter(s.storer, s.post, s.signer, batch)
	if err != nil {
		s.logger.Debugf("chunk upload: putter:%v", err)
		s.logger.Error("chunk upload: putter")
		jsonhttp.BadRequest(w, nil)
		return
	}

	seen, err := putter.Put(ctx, requestModePut(r), chunk)
	if err != nil {
		s.logger.Debugf("chunk upload: chunk write error: %v, addr %s", err, chunk.Address())
		s.logger.Error("chunk upload: chunk write error")
		jsonhttp.BadRequest(w, "chunk write error")
		return
	} else if len(seen) > 0 && seen[0] && tag != nil {
		err := tag.Inc(tags.StateSeen)
		if err != nil {
			s.logger.Debugf("chunk upload: increment tag", err)
			s.logger.Error("chunk upload: increment tag")
			jsonhttp.BadRequest(w, "increment tag")
			return
		}
	}

	if tag != nil {
		// indicate that the chunk is stored
		err = tag.Inc(tags.StateStored)
		if err != nil {
			s.logger.Debugf("chunk upload: increment tag", err)
			s.logger.Error("chunk upload: increment tag")
			jsonhttp.InternalServerError(w, "increment tag")
			return
		}
		w.Header().Set(SwarmTagHeader, fmt.Sprint(tag.Uid))
	}

	if strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true" {
		if err := s.pinning.CreatePin(ctx, chunk.Address(), false); err != nil {
			s.logger.Debugf("chunk upload: creation of pin for %q failed: %v", chunk.Address(), err)
			s.logger.Error("chunk upload: creation of pin failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
	}

	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, chunkAddressResponse{Reference: chunk.Address()})
}

func (s *server) chunkGetHandler(w http.ResponseWriter, r *http.Request) {
	targets := r.URL.Query().Get("targets")
	if targets != "" {
		r = r.WithContext(sctx.SetTargets(r.Context(), targets))
	}

	nameOrHex := mux.Vars(r)["addr"]
	ctx := r.Context()

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		s.logger.Debugf("chunk: parse chunk address %s: %v", nameOrHex, err)
		s.logger.Error("chunk: parse chunk address error")
		jsonhttp.NotFound(w, nil)
		return
	}

	chunk, err := s.storer.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.logger.Tracef("chunk: chunk not found. addr %s", address)
			jsonhttp.NotFound(w, "chunk not found")
			return

		}
		if errors.Is(err, netstore.ErrRecoveryAttempt) {
			s.logger.Tracef("chunk: chunk recovery initiated. addr %s", address)
			jsonhttp.Accepted(w, "chunk recovery initiated. retry after sometime.")
			return
		}
		s.logger.Debugf("chunk: chunk read error: %v ,addr %s", err, address)
		s.logger.Error("chunk: chunk read error")
		jsonhttp.InternalServerError(w, "chunk read error")
		return
	}
	w.Header().Set("Content-Type", "binary/octet-stream")
	if targets != "" {
		w.Header().Set(TargetsRecoveryHeader, targets)
	}
	_, _ = io.Copy(w, bytes.NewReader(chunk.Data()))
}

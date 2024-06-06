// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/soc"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

type chunkAddressResponse struct {
	Reference swarm.Address `json:"reference"`
}

func (s *Service) chunkUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_chunk").Build()

	headers := struct {
		BatchID        []byte        `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag       uint64        `map:"Swarm-Tag"`
		Act            bool          `map:"Swarm-Act"`
		HistoryAddress swarm.Address `map:"Swarm-Act-History-Address"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	var (
		tag uint64
		err error
	)
	if headers.SwarmTag > 0 {
		tag, err = s.getOrCreateSessionID(headers.SwarmTag)
		if err != nil {
			logger.Debug("get or create tag failed", "error", err)
			logger.Error(nil, "get or create tag failed")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "cannot get or create tag")
			}
			return
		}
	}

	// Currently the localstore supports session based uploads. We don't want to
	// create new session for single chunk uploads. So if the chunk upload is not
	// part of a session already, then we directly push the chunk. This way we dont
	// need to go through the UploadStore.
	deferred := tag != 0

	putter, err := s.newStamperPutter(r.Context(), putterOptions{
		BatchID:  headers.BatchID,
		TagID:    tag,
		Deferred: deferred,
	})
	if err != nil {
		logger.Debug("get putter failed", "error", err)
		logger.Error(nil, "get putter failed")
		switch {
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		case errors.Is(err, errUnsupportedDevNodeOperation):
			jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		default:
			jsonhttp.BadRequest(w, nil)
		}
		return
	}

	ow := &cleanupOnErrWriter{
		ResponseWriter: w,
		onErr:          putter.Cleanup,
		logger:         logger,
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, ow) {
			return
		}
		logger.Debug("chunk upload: read chunk data failed", "error", err)
		logger.Error(nil, "chunk upload: read chunk data failed")
		jsonhttp.InternalServerError(ow, "cannot read chunk data")
		return
	}

	if len(data) < swarm.SpanSize {
		logger.Debug("chunk upload: insufficient data length")
		logger.Error(nil, "chunk upload: insufficient data length")
		jsonhttp.BadRequest(ow, "insufficient data length")
		return
	}

	chunk, err := cac.NewWithDataSpan(data)
	if err != nil {
		// not a valid cac chunk. Check if it's a replica soc chunk.
		logger.Debug("chunk upload: create chunk failed", "error", err)

		// FromChunk only uses the chunk data to recreate the soc chunk. So the address is irrelevant.
		sch, err := soc.FromChunk(swarm.NewChunk(swarm.EmptyAddress, data))
		if err != nil {
			logger.Debug("chunk upload: create soc chunk from data failed", "error", err)
			logger.Error(nil, "chunk upload: create chunk error")
			jsonhttp.InternalServerError(ow, "create chunk error")
			return
		}
		chunk, err = sch.Chunk()
		if err != nil {
			logger.Debug("chunk upload: create chunk from soc failed", "error", err)
			logger.Error(nil, "chunk upload: create chunk error")
			jsonhttp.InternalServerError(ow, "create chunk error")
			return
		}

		if !soc.Valid(chunk) {
			logger.Debug("chunk upload: invalid soc chunk")
			logger.Error(nil, "chunk upload: create chunk error")
			jsonhttp.InternalServerError(ow, "create chunk error")
			return
		}
	}

	reference := chunk.Address()
	if headers.Act {
		reference, err = s.actEncryptionHandler(r.Context(), w, putter, reference, headers.HistoryAddress)
		if err != nil {
			jsonhttp.InternalServerError(w, errActUpload)
			return
		}
	}

	err = putter.Put(r.Context(), chunk)
	if err != nil {
		logger.Debug("chunk upload: write chunk failed", "chunk_address", chunk.Address(), "error", err)
		logger.Error(nil, "chunk upload: write chunk failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(ow, "batch is overissued")
		default:
			jsonhttp.InternalServerError(ow, "chunk write error")
		}
		return
	}

	err = putter.Done(swarm.ZeroAddress)
	if err != nil {
		logger.Debug("done split failed", "error", err)
		logger.Error(nil, "done split failed")
		jsonhttp.InternalServerError(ow, "done split failed")
		return
	}

	if tag != 0 {
		w.Header().Set(SwarmTagHeader, fmt.Sprint(tag))
	}

	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, chunkAddressResponse{Reference: reference})
}

func (s *Service) chunkGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_chunk_by_address").Build()
	loggerV1 := logger.V(1).Build()

	headers := struct {
		Cache *bool `map:"Swarm-Cache"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}
	cache := true
	if headers.Cache != nil {
		cache = *headers.Cache
	}

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

	chunk, err := s.storer.Download(cache).Get(r.Context(), address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			loggerV1.Debug("chunk not found", "address", address)
			jsonhttp.NotFound(w, "chunk not found")
			return

		}
		logger.Debug("read chunk failed", "chunk_address", address, "error", err)
		logger.Error(nil, "read chunk failed")
		jsonhttp.InternalServerError(w, "read chunk failed")
		return
	}
	w.Header().Set(ContentTypeHeader, "binary/octet-stream")
	w.Header().Set(ContentLengthHeader, strconv.FormatInt(int64(len(chunk.Data())), 10))
	_, _ = io.Copy(w, bytes.NewReader(chunk.Data()))
}

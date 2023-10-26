// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *Service) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("post_bytes").Build())

	headers := struct {
		BatchID  []byte `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag uint64 `map:"Swarm-Tag"`
		Pin      bool   `map:"Swarm-Pin"`
		Deferred *bool  `map:"Swarm-Deferred-Upload"`
		Encrypt  bool   `map:"Swarm-Encrypt"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	var (
		tag      uint64
		err      error
		deferred = defaultUploadMethod(headers.Deferred)
	)

	if deferred || headers.Pin {
		tag, err = s.getOrCreateSessionID(headers.SwarmTag)
		if err != nil {
			logger.Debug("unable to get/create session", "tag", headers.SwarmTag, "error", err)
			logger.Warning("unable to get/create session")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "get/create tag")
			}
			return
		}
	}

	putter, err := s.newStamperPutter(r.Context(), putterOptions{
		BatchID:  headers.BatchID,
		TagID:    tag,
		Pin:      headers.Pin,
		Deferred: deferred,
	})
	if err != nil {
		logger.Debug("unable to get putter", "batch_id", headers.BatchID, "tag_id", tag, "pin", headers.Pin, "deferred", deferred, "error", err)
		logger.Warning("unable to get putter")
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

	p := requestPipelineFn(putter, headers.Encrypt)
	address, err := p(r.Context(), r.Body)
	if err != nil {
		logger.Debug("unable to write to the pipeline", "encrypt", headers.Encrypt, "error", err)
		logger.Warning("unable to write to the pipeline")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(ow, "batch is over-issued")
		default:
			jsonhttp.InternalServerError(ow, "unable to write to pipeline")
		}
		return
	}

	err = putter.Done(address)
	if err != nil {
		logger.Debug("unable to close session", "tag_id", tag, "error", err)
		logger.Warning("unable to close session")
		jsonhttp.InternalServerError(ow, "unable to close session")
		return
	}

	if tag != 0 {
		w.Header().Set(SwarmTagHeader, fmt.Sprint(tag))
	}
	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, bytesPostResponse{
		Reference: address,
	})
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

	additionalHeaders := http.Header{
		ContentTypeHeader: {"application/octet-stream"},
	}

	s.downloadHandler(logger, w, r, paths.Address, additionalHeaders, true)
}

func (s *Service) bytesHeadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("head_bytes_by_address").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	ch, err := s.storer.Download(true).Get(r.Context(), paths.Address)
	if err != nil {
		logger.Debug("unable to get root chunk", "chunk_address", paths.Address, "error", err)
		logger.Warning("unable to get root chunk")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Add("Access-Control-Expose-Headers", "Accept-Ranges, Content-Encoding")
	w.Header().Add(ContentTypeHeader, "application/octet-stream")
	var span int64

	if cac.Valid(ch) {
		span = int64(binary.LittleEndian.Uint64(ch.Data()[:swarm.SpanSize]))
	} else {
		// soc
		span = int64(len(ch.Data()))
	}
	w.Header().Set(ContentLengthHeader, strconv.FormatInt(span, 10))
	w.WriteHeader(http.StatusOK) // HEAD requests do not write a body
}

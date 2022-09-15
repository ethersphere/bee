// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/util/ioutil"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *Service) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)

	putter, wait, err := s.newStamperPutter(r)
	if err != nil {
		logger.Debug("bytes upload: putter failed", "error", err)
		logger.Error(nil, "bytes upload: putter failed")
		switch {
		case errors.Is(err, storage.ErrNotFound):
			jsonhttp.NotFound(w, "batch not found")
		case errors.Is(err, errBatchUnusable):
			jsonhttp.BadRequest(w, "batch not usable yet")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.NotFound(w, "invalid batch id")
		default:
			jsonhttp.InternalServerError(w, nil)
		}
		return
	}

	if strings.Contains(strings.ToLower(r.Header.Get(contentTypeHeader)), "multipart/form-data") {
		logger.Error(nil, "bytes upload: multipart uploads are not supported on this endpoint")
		jsonhttp.BadRequest(w, "multipart uploads not supported")
		return
	}

	tag, created, err := s.getOrCreateTag(r.Header.Get(SwarmTagHeader))
	if err != nil {
		logger.Debug("bytes upload: get or create tag failed", "error", err)
		logger.Error(nil, "bytes upload: get or create tag failed")
		switch {
		case errors.Is(err, tags.ErrExists):
			jsonhttp.Conflict(w, "bytes upload: conflict with current state of resource")
		case errors.Is(err, errCannotParse):
			jsonhttp.BadRequest(w, "bytes upload: request cannot be parsed")
		case errors.Is(err, tags.ErrNotFound):
			jsonhttp.NotFound(w, "bytes upload: not found")
		default:
			jsonhttp.InternalServerError(w, "cannot get or create tag")
		}
		return
	}

	if !created {
		// only in the case when tag is sent via header (i.e. not created by this request)
		if estimatedTotalChunks := requestCalculateNumberOfChunks(r); estimatedTotalChunks > 0 {
			err = tag.IncN(tags.TotalChunks, estimatedTotalChunks)
			if err != nil {
				s.logger.Debug("bytes upload: increment tag failed", "error", err)
				s.logger.Error(nil, "bytes upload: increment tag failed")
				jsonhttp.InternalServerError(w, "increment tag")
				return
			}
		}
	}

	// Add the tag to the context
	ctx := sctx.SetTag(r.Context(), tag)
	p := requestPipelineFn(putter, r)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pr := ioutil.TimeoutReader(ctx, r.Body, time.Minute, func(n uint64) {
		logger.Error(nil, "bytes upload: idle read timeout exceeded")
		logger.Debug("bytes upload: idle read timeout exceeded", "bytes_read", n)
		cancel()
	})
	address, err := p(ctx, pr)
	if err != nil {
		logger.Debug("bytes upload: split write all failed", "error", err)
		logger.Error(nil, "bytes upload: split write all failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "bytes upload: split write all failed")
		}
		return
	}
	if err = wait(); err != nil {
		logger.Debug("bytes upload: sync chunks failed", "error", err)
		logger.Error(nil, "bytes upload: sync chunks failed")
		jsonhttp.InternalServerError(w, "bytes upload: sync chunks failed")
		return
	}

	if created {
		_, err = tag.DoneSplit(address)
		if err != nil {
			logger.Debug("bytes upload: done split failed", "error", err)
			logger.Error(nil, "bytes upload: done split failed")
			jsonhttp.InternalServerError(w, "bytes upload: done split filed")
			return
		}
	}

	if strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true" {
		if err := s.pinning.CreatePin(ctx, address, false); err != nil {
			logger.Debug("bytes upload: pin creation failed", "address", address, "error", err)
			logger.Error(nil, "bytes upload: pin creation failed")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "bytes upload: create pin failed: not found")
			default:
				jsonhttp.InternalServerError(w, "bzz upload: create pin failed")
			}
			return
		}
	}

	w.Header().Set(SwarmTagHeader, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, bytesPostResponse{
		Reference: address,
	})
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *Service) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	nameOrHex := mux.Vars(r)["address"]

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debug("bytes: parse address string failed", nameOrHex, err)
		logger.Error(nil, "bytes: parse address string failed")
		jsonhttp.NotFound(w, nil)
		return
	}

	additionalHeaders := http.Header{
		"Content-Type": {"application/octet-stream"},
	}

	s.downloadHandler(w, r, address, additionalHeaders, true)
}

func (s *Service) bytesHeadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	nameOrHex := mux.Vars(r)["address"]

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debug("bytes: parse address string failed", "string", nameOrHex, "error", err)
		logger.Error(nil, "bytes: parse address string failed")
		w.WriteHeader(http.StatusBadRequest) // HEAD requests do not write a body
		return
	}
	ch, err := s.storer.Get(r.Context(), storage.ModeGetRequest, address)
	if err != nil {
		logger.Debug("bytes: get root chunk failed", "chunk_address", address, "error", err)
		logger.Error(nil, "bytes: get rook chunk failed")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Add("Access-Control-Expose-Headers", "Accept-Ranges, Content-Encoding")
	w.Header().Add("Content-Type", "application/octet-stream")
	var span int64

	if cac.Valid(ch) {
		span = int64(binary.LittleEndian.Uint64(ch.Data()[:swarm.SpanSize]))
	} else {
		// soc
		span = int64(len(ch.Data()))
	}
	w.Header().Add("Content-Length", strconv.FormatInt(span, 10))
	w.WriteHeader(http.StatusOK) // HEAD requests do not write a body
}

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
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
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
		ContentType string `map:"Content-Type" validate:"excludes=multipart/form-data"`
		SwarmTag    string `map:"Swarm-Tag"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	putter, wait, err := s.newStamperPutter(r)
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

	tag, created, err := s.getOrCreateTag(headers.SwarmTag)
	if err != nil {
		logger.Debug("get or create tag failed", "error", err)
		logger.Error(nil, "get or create tag failed")
		switch {
		case errors.Is(err, tags.ErrNotFound):
			jsonhttp.NotFound(w, "tag not found")
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
				logger.Debug("increment tag failed", "error", err)
				logger.Error(nil, "increment tag failed")
				jsonhttp.InternalServerError(w, "increment tag failed")
				return
			}
		}
	}

	// Add the tag to the context
	ctx := sctx.SetTag(r.Context(), tag)
	p := requestPipelineFn(putter, r)
	address, err := p(ctx, r.Body)
	if err != nil {
		logger.Debug("split write all failed", "error", err)
		logger.Error(nil, "split write all failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "split write all failed")
		}
		return
	}
	if err = wait(); err != nil {
		logger.Debug("sync chunks failed", "error", err)
		logger.Error(nil, "sync chunks failed")
		jsonhttp.InternalServerError(w, "sync chunks failed")
		return
	}

	if created {
		_, err = tag.DoneSplit(address)
		if err != nil {
			logger.Debug("done split failed", "error", err)
			logger.Error(nil, "done split failed")
			jsonhttp.InternalServerError(w, "done split filed")
			return
		}
	}

	if requestPin(r) {
		if err := s.pinning.CreatePin(ctx, address, false); err != nil {
			logger.Debug("pin creation failed", "address", address, "error", err)
			logger.Error(nil, "pin creation failed")
			jsonhttp.InternalServerError(w, "create ping failed")
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

	ch, err := s.storer.Get(r.Context(), storage.ModeGetRequest, paths.Address)
	if err != nil {
		logger.Debug("get root chunk failed", "chunk_address", paths.Address, "error", err)
		logger.Error(nil, "get rook chunk failed")
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

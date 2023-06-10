// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/mux"
)

type chunkAddressResponse struct {
	Reference swarm.Address `json:"reference"`
}

func (s *Service) processUploadRequest(
	logger log.Logger, r *http.Request,
) (ctx context.Context, tag *tags.Tag, putter storage.Putter, waitFn func() error, err error) {

	if str := r.Header.Get(SwarmTagHeader); str != "" {
		tag, err = s.getTag(str)
		if err != nil {
			logger.Debug("get tag failed", "string", str, "error", err)
			logger.Error(nil, "get tag failed", "string", str)
			return nil, nil, nil, nil, errors.New("cannot get tag")
		}

		// add the tag to the context if it exists
		ctx = sctx.SetTag(r.Context(), tag)
	} else {
		ctx = r.Context()
	}

	// if the header SwarmPostageStampHeader is set, we use newStampedPutter,
	// otherwise we use the newStamperPutter.
	if str := r.Header.Get(SwarmPostageStampHeader); str != "" {
		putter, wait, err := s.newStampedPutter(r)
		if err != nil {
			logger.Debug("putter failed", "error", err)
			logger.Error(nil, "putter failed")
			return nil, nil, nil, nil, err
		}

		return ctx, tag, putter, wait, nil
	}

	putter, wait, err := s.newStamperPutter(r)
	if err != nil {
		logger.Debug("putter failed", "error", err)
		logger.Error(nil, "putter failed")
		return nil, nil, nil, nil, err
	}

	return ctx, tag, putter, wait, nil
}

func (s *Service) chunkUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_chunk").Build()

	ctx, tag, putter, wait, err := s.processUploadRequest(logger, r)
	if err != nil {
		switch {
		case errors.Is(err, tags.ErrNotFound):
			jsonhttp.NotFound(w, "tag not found")
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		case errors.Is(err, errInvalidPostageStamp):
			jsonhttp.BadRequest(w, "invalid postage stamp")
		case errors.Is(err, errUnsupportedDevNodeOperation):
			jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		default:
			jsonhttp.BadRequest(w, nil)
		}
		return
	}

	if tag != nil {
		err = tag.Inc(tags.StateSplit)
		if err != nil {
			s.logger.Debug("chunk upload: increment tag failed", "error", err)
			s.logger.Error(nil, "chunk upload: increment tag failed")
			jsonhttp.InternalServerError(w, "increment tag")
			return
		}
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debug("chunk upload: read chunk data failed", "error", err)
		s.logger.Error(nil, "chunk upload: read chunk data failed")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if len(data) < swarm.SpanSize {
		s.logger.Debug("chunk upload: insufficient data length")
		s.logger.Error(nil, "chunk upload: insufficient data length")
		jsonhttp.BadRequest(w, "insufficient data length")
		return
	}

	chunk, err := cac.NewWithDataSpan(data)
	if err != nil {
		s.logger.Debug("chunk upload: create chunk failed", "error", err)
		s.logger.Error(nil, "chunk upload: create chunk error")
		jsonhttp.InternalServerError(w, "create chunk error")
		return
	}

	seen, err := putter.Put(ctx, requestModePut(r), chunk)
	if err != nil {
		s.logger.Debug("chunk upload: write chunk failed", "chunk_address", chunk.Address(), "error", err)
		s.logger.Error(nil, "chunk upload: write chunk failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "chunk write error")
		}
		return
	} else if len(seen) > 0 && seen[0] && tag != nil {
		err := tag.Inc(tags.StateSeen)
		if err != nil {
			s.logger.Debug("chunk upload: increment tag failed", "error", err)
			s.logger.Error(nil, "chunk upload: increment tag failed")
			jsonhttp.BadRequest(w, "increment tag")
			return
		}
	}

	if tag != nil {
		// indicate that the chunk is stored
		err = tag.Inc(tags.StateStored)
		if err != nil {
			s.logger.Debug("chunk upload: increment tag failed", "error", err)
			s.logger.Error(nil, "chunk upload: increment tag failed")
			jsonhttp.InternalServerError(w, "increment tag failed")
			return
		}
		w.Header().Set(SwarmTagHeader, fmt.Sprint(tag.Uid))
	}

	if requestPin(r) {
		if err := s.pinning.CreatePin(ctx, chunk.Address(), false); err != nil {
			s.logger.Debug("chunk upload: pin creation failed", "chunk_address", chunk.Address(), "error", err)
			s.logger.Error(nil, "chunk upload: pin creation failed")
			err = s.storer.Set(ctx, storage.ModeSetUnpin, chunk.Address())
			if err != nil {
				s.logger.Debug("chunk upload: pin deletion failed", "chunk_address", chunk.Address(), "error", err)
				s.logger.Error(nil, "chunk upload: pin deletion failed")
			}
			jsonhttp.InternalServerError(w, "creation of pin failed")
			return
		}
	}

	if err = wait(); err != nil {
		s.logger.Debug("chunk upload: sync chunk failed", "error", err)
		switch {
		case errors.Is(err, errUnsupportedDevNodeOperation):
			s.logger.Error(err, "chunk upload: direct upload not supported in dev mode")
			jsonhttp.BadRequest(w, "dev mode does not support this operation")
		default:
			s.logger.Error(err, "chunk upload: sync chunk failed")
			jsonhttp.InternalServerError(w, "sync failed")
		}
		return
	}

	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, chunkAddressResponse{Reference: chunk.Address()})
}

func (s *Service) chunkGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_chunk_by_address").Build()
	loggerV1 := logger.V(1).Build()

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	chunk, err := s.storer.Get(r.Context(), storage.ModeGetRequest, paths.Address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			loggerV1.Debug("chunk not found", "address", paths.Address)
			jsonhttp.NotFound(w, "chunk not found")
			return

		}
		logger.Debug("read chunk failed", "chunk_address", paths.Address, "error", err)
		logger.Error(nil, "read chunk failed")
		jsonhttp.InternalServerError(w, "read chunk failed")
		return
	}
	w.Header().Set("Content-Type", "binary/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(chunk.Data())), 10))
	_, _ = io.Copy(w, bytes.NewReader(chunk.Data()))
}

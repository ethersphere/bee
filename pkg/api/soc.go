// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"errors"
	"io"
	"net/http"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type socPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

func (s *Service) socUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_soc").Build()

	paths := struct {
		Owner []byte `map:"owner" validate:"required"`
		ID    []byte `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	queries := struct {
		Sig []byte `map:"sig" validate:"required"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		logger.Debug("read body failed", "error", err)
		logger.Error(nil, "read body failed")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if len(data) < swarm.SpanSize {
		logger.Debug("chunk data too short")
		logger.Error(nil, "chunk data too short")
		jsonhttp.BadRequest(w, "short chunk data")
		return
	}

	if len(data) > swarm.ChunkSize+swarm.SpanSize {
		logger.Debug("chunk data exceeds required length", "required_length", swarm.ChunkSize+swarm.SpanSize)
		logger.Error(nil, "chunk data exceeds required length")
		jsonhttp.RequestEntityTooLarge(w, "payload too large")
		return
	}

	ch, err := cac.NewWithDataSpan(data)
	if err != nil {
		logger.Debug("create content addressed chunk failed", "error", err)
		logger.Error(nil, "create content addressed chunk failed")
		jsonhttp.BadRequest(w, "chunk data error")
		return
	}

	ss, err := soc.NewSigned(paths.ID, ch, paths.Owner, queries.Sig)
	if err != nil {
		logger.Debug("create soc failed", "id", paths.ID, "owner", paths.Owner, "error", err)
		logger.Error(nil, "create soc failed")
		jsonhttp.Unauthorized(w, "invalid address")
		return
	}

	sch, err := ss.Chunk()
	if err != nil {
		logger.Debug("read chunk data failed", "error", err)
		logger.Error(nil, "read chunk data failed")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if !soc.Valid(sch) {
		logger.Debug("invalid chunk", "error", err)
		logger.Error(nil, "invalid chunk")
		jsonhttp.Unauthorized(w, "invalid chunk")
		return
	}

	ctx := r.Context()

	has, err := s.storer.Has(ctx, sch.Address())
	if err != nil {
		logger.Debug("has check failed", "chunk_address", sch.Address(), "error", err)
		logger.Error(nil, "has check failed")
		jsonhttp.InternalServerError(w, "storage error")
		return
	}
	if has {
		logger.Error(nil, "chunk already exists")
		jsonhttp.Conflict(w, "chunk already exists")
		return
	}
	batch, err := requestPostageBatchId(r)
	if err != nil {
		logger.Debug("mapStructure postage batch id failed", "error", err)
		logger.Error(nil, "mapStructure postage batch id failed")
		jsonhttp.BadRequest(w, "invalid postage batch id")
		return
	}

	i, save, err := s.post.GetStampIssuer(batch)
	if err != nil {
		logger.Debug("get postage batch issuer failed", "batch_id", hex.EncodeToString(batch), "error", err)
		logger.Error(nil, "get postage batch issue")
		switch {
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.BadRequest(w, "batch not found")
		case errors.Is(err, postage.ErrNotUsable):
			jsonhttp.BadRequest(w, "batch not usable yet")
		default:
			jsonhttp.BadRequest(w, "postage stamp issuer")
		}
		return
	}
	defer func() {
		if err := save(); err != nil {
			s.logger.Debug("stamp issuer save", "error", err)
		}
	}()

	stamper := postage.NewStamper(i, s.signer)
	stamp, err := stamper.Stamp(sch.Address())
	if err != nil {
		logger.Debug("stamp failed", "chunk_address", sch.Address(), "error", err)
		logger.Error(nil, "stamp failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "stamp error")
		}
		return
	}
	sch = sch.WithStamp(stamp)
	_, err = s.storer.Put(ctx, requestModePut(r), sch)
	if err != nil {
		logger.Debug("write chunk failed", "chunk_address", sch.Address(), "error", err)
		logger.Error(nil, "write chunk failed")
		jsonhttp.BadRequest(w, "chunk write error")
		return
	}

	if requestPin(r) {
		if err := s.pinning.CreatePin(ctx, sch.Address(), false); err != nil {
			logger.Debug("create pin failed", "chunk_address", sch.Address(), "error", err)
			logger.Error(nil, "create pin failed")
			jsonhttp.InternalServerError(w, "creation of pin failed")
			return
		}
	}

	jsonhttp.Created(w, chunkAddressResponse{Reference: sch.Address()})
}

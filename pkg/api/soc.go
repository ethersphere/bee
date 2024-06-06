// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/soc"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

	headers := struct {
		BatchID        []byte        `map:"Swarm-Postage-Batch-Id"`
		StampSig       []byte        `map:"Swarm-Postage-Stamp"`
		Pin            bool          `map:"Swarm-Pin"`
		Act            bool          `map:"Swarm-Act"`
		HistoryAddress swarm.Address `map:"Swarm-Act-History-Address"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	if len(headers.BatchID) == 0 && len(headers.StampSig) == 0 {
		logger.Error(nil, batchIdOrStampSig)
		jsonhttp.BadRequest(w, batchIdOrStampSig)
		return
	}

	// if pinning header is set we do a deferred upload, else we do a direct upload
	var (
		tag uint64
		err error
	)
	if headers.Pin {
		session, err := s.storer.NewSession()
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
		tag = session.TagID
	}

	deferred := tag != 0

	var putter storer.PutterSession
	if len(headers.StampSig) != 0 {
		stamp := postage.Stamp{}
		if err := stamp.UnmarshalBinary(headers.StampSig); err != nil {
			errorMsg := "Stamp deserialization failure"
			logger.Debug(errorMsg, "error", err)
			logger.Error(nil, errorMsg)
			jsonhttp.BadRequest(w, errorMsg)
			return
		}

		putter, err = s.newStampedPutter(r.Context(), putterOptions{
			BatchID:  stamp.BatchID(),
			TagID:    tag,
			Pin:      headers.Pin,
			Deferred: deferred,
		}, &stamp)
	} else {
		putter, err = s.newStamperPutter(r.Context(), putterOptions{
			BatchID:  headers.BatchID,
			TagID:    tag,
			Pin:      headers.Pin,
			Deferred: deferred,
		})
	}
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
		logger.Debug("read body failed", "error", err)
		logger.Error(nil, "read body failed")
		jsonhttp.InternalServerError(ow, "cannot read chunk data")
		return
	}

	if len(data) < swarm.SpanSize {
		logger.Debug("chunk data too short")
		logger.Error(nil, "chunk data too short")
		jsonhttp.BadRequest(ow, "short chunk data")
		return
	}

	if len(data) > swarm.ChunkSize+swarm.SpanSize {
		logger.Debug("chunk data exceeds required length", "required_length", swarm.ChunkSize+swarm.SpanSize)
		logger.Error(nil, "chunk data exceeds required length")
		jsonhttp.RequestEntityTooLarge(ow, "payload too large")
		return
	}

	ch, err := cac.NewWithDataSpan(data)
	if err != nil {
		logger.Debug("create content addressed chunk failed", "error", err)
		logger.Error(nil, "create content addressed chunk failed")
		jsonhttp.BadRequest(ow, "chunk data error")
		return
	}

	ss, err := soc.NewSigned(paths.ID, ch, paths.Owner, queries.Sig)
	if err != nil {
		logger.Debug("create soc failed", "id", paths.ID, "owner", paths.Owner, "error", err)
		logger.Error(nil, "create soc failed")
		jsonhttp.Unauthorized(ow, "invalid address")
		return
	}

	sch, err := ss.Chunk()
	if err != nil {
		logger.Debug("read chunk data failed", "error", err)
		logger.Error(nil, "read chunk data failed")
		jsonhttp.InternalServerError(ow, "cannot read chunk data")
		return
	}

	if !soc.Valid(sch) {
		logger.Debug("invalid chunk", "error", err)
		logger.Error(nil, "invalid chunk")
		jsonhttp.Unauthorized(ow, "invalid chunk")
		return
	}

	reference := sch.Address()
	if headers.Act {
		reference, err = s.actEncryptionHandler(r.Context(), w, putter, reference, headers.HistoryAddress)
		if err != nil {
			jsonhttp.InternalServerError(w, errActUpload)
			return
		}
	}

	err = putter.Put(r.Context(), sch)
	if err != nil {
		logger.Debug("write chunk failed", "chunk_address", sch.Address(), "error", err)
		logger.Error(nil, "write chunk failed")
		jsonhttp.BadRequest(ow, "chunk write error")
		return
	}

	err = putter.Done(sch.Address())
	if err != nil {
		logger.Debug("done split failed", "error", err)
		logger.Error(nil, "done split failed")
		jsonhttp.InternalServerError(ow, "done split failed")
		return
	}

	jsonhttp.Created(w, socPostResponse{Reference: reference})
}

// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storer"
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

	var (
		putter storer.PutterSession
		err    error
	)

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
			TagID:    0,
			Pin:      false,
			Deferred: false,
		}, &stamp)
	} else {
		putter, err = s.newStamperPutter(r.Context(), putterOptions{
			BatchID:  headers.BatchID,
			TagID:    0,
			Pin:      false,
			Deferred: false,
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
		case errors.Is(err, errUnsupportedDevNodeOperation):
			jsonhttp.NotImplemented(w, "operation is not supported in dev mode")
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
			logger.Debug("access control upload failed", "error", err)
			logger.Error(nil, "access control upload failed")
			switch {
			case errors.Is(err, accesscontrol.ErrNotFound):
				jsonhttp.NotFound(w, "act or history entry not found")
			case errors.Is(err, accesscontrol.ErrInvalidPublicKey) || errors.Is(err, accesscontrol.ErrSecretKeyInfinity):
				jsonhttp.BadRequest(w, "invalid public key")
			case errors.Is(err, accesscontrol.ErrUnexpectedType):
				jsonhttp.BadRequest(w, "failed to create history")
			default:
				jsonhttp.InternalServerError(w, errActUpload)
			}
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

func (s *Service) socGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_soc").Build()

	paths := struct {
		Owner []byte `map:"owner" validate:"required"`
		ID    []byte `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		OnlyRootChunk bool `map:"Swarm-Only-Root-Chunk"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	address, err := soc.CreateAddress(paths.ID, paths.Owner)
	if err != nil {
		logger.Error(err, "soc address cannot be created")
		jsonhttp.BadRequest(w, "soc address cannot be created")
		return
	}

	getter := s.storer.Download(true)
	sch, err := getter.Get(r.Context(), address)
	if err != nil {
		logger.Error(err, "soc retrieval has been failed")
		jsonhttp.NotFound(w, "requested chunk cannot be retrieved")
		return
	}
	socCh, err := soc.FromChunk(sch)
	if err != nil {
		logger.Error(err, "chunk is not a single owner chunk")
		jsonhttp.InternalServerError(w, "chunk is not a single owner chunk")
		return
	}

	sig := socCh.Signature()
	wc := socCh.WrappedChunk()

	additionalHeaders := http.Header{
		ContentTypeHeader:          {"application/octet-stream"},
		SwarmSocSignatureHeader:    {hex.EncodeToString(sig)},
		AccessControlExposeHeaders: {SwarmSocSignatureHeader},
	}

	if headers.OnlyRootChunk {
		w.Header().Set(ContentLengthHeader, strconv.Itoa(len(wc.Data())))
		// include additional headers
		for name, values := range additionalHeaders {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		_, _ = io.Copy(w, bytes.NewReader(wc.Data()))
		return
	}

	s.downloadHandler(logger, w, r, wc.Address(), additionalHeaders, true, false, wc)
}

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
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/replicas"
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
		BatchID        []byte            `map:"Swarm-Postage-Batch-Id"`
		StampSig       []byte            `map:"Swarm-Postage-Stamp"`
		Act            bool              `map:"Swarm-Act"`
		HistoryAddress swarm.Address     `map:"Swarm-Act-History-Address"`
		RLevel         *redundancy.Level `map:"Swarm-Redundancy-Level"`
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
		basePutter storer.PutterSession // the putter used to store regular chunks
		putter     storer.PutterSession // the putter used to store SOC replica chunks
		err        error
	)

	rLevel := getRedundancyLevel(headers.RLevel)

	if len(headers.StampSig) != 0 {
		if headers.RLevel != nil {
			logger.Error(nil, "redundancy level is not supported with stamp signature")
			jsonhttp.BadRequest(w, "redundancy level is not supported with stamp signature")
			return
		}
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
		basePutter = putter
	} else {
		putter, err = s.newStamperPutter(r.Context(), putterOptions{
			BatchID:  headers.BatchID,
			TagID:    0,
			Pin:      false,
			Deferred: false,
		})
		basePutter = putter
		if rLevel != redundancy.NONE {
			putter = replicas.NewSocPutterSession(putter, rLevel)
		}
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

	err = putter.Put(r.Context(), sch)
	if err != nil {
		logger.Debug("write chunk failed", "chunk_address", sch.Address(), "error", err)
		logger.Error(nil, "write chunk failed")
		jsonhttp.BadRequest(ow, "chunk write error")
		return
	}

	reference := sch.Address()
	historyReference := swarm.ZeroAddress
	if headers.Act {
		reference, historyReference, err = s.actEncryptionHandler(r.Context(), basePutter, reference, headers.HistoryAddress)
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

	// do not pass sch.Address() since it causes error on parallel GSOC uploads
	// in case of deferred upload
	// pkg/storer/internal/pinning/pinning.go:collectionPutter.Close -> throws error if pin true but that is not a valid use-case at SOC upload
	// pkg/storer/internal/upload/uploadstore.go:uploadPutter.Close -> updates tagID, and the address would be set along with it -> not necessary
	// in case of directupload it only waits for the waitgroup for chunk upload and do not use swarm address
	err = putter.Done(swarm.Address{})
	if err != nil {
		logger.Debug("done split failed", "error", err)
		logger.Error(nil, "done split failed")
		jsonhttp.InternalServerError(ow, "done split failed")
		return
	}
	if headers.Act {
		w.Header().Set(SwarmActHistoryAddressHeader, historyReference.String())
		w.Header().Set(AccessControlExposeHeaders, SwarmActHistoryAddressHeader)
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
		OnlyRootChunk bool              `map:"Swarm-Only-Root-Chunk"`
		RLevel        *redundancy.Level `map:"Swarm-Redundancy-Level"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	rLevel := getRedundancyLevel(headers.RLevel)

	address, err := soc.CreateAddress(paths.ID, paths.Owner)
	if err != nil {
		logger.Error(err, "soc address cannot be created")
		jsonhttp.BadRequest(w, "soc address cannot be created")
		return
	}

	getter := s.storer.Download(true)
	if rLevel > redundancy.NONE {
		getter = replicas.NewSocGetter(getter, rLevel)
	}
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

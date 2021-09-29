// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

var errBadRequestParams = errors.New("owner, id or span is not well formed")

type socPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

func (s *server) socUploadHandler(w http.ResponseWriter, r *http.Request) {
	owner, err := hex.DecodeString(mux.Vars(r)["owner"])
	if err != nil {
		s.logger.Debugf("soc upload: bad owner: %v", err)
		s.logger.Error("soc upload: %v", errBadRequestParams)
		jsonhttp.BadRequest(w, "bad owner")
		return
	}
	id, err := hex.DecodeString(mux.Vars(r)["id"])
	if err != nil {
		s.logger.Debugf("soc upload: bad id: %v", err)
		s.logger.Error("soc upload: %v", errBadRequestParams)
		jsonhttp.BadRequest(w, "bad id")
		return
	}

	sigStr := r.URL.Query().Get("sig")
	if sigStr == "" {
		s.logger.Debugf("soc upload: empty signature")
		s.logger.Error("soc upload: empty signature")
		jsonhttp.BadRequest(w, "empty signature")
		return
	}

	sig, err := hex.DecodeString(sigStr)
	if err != nil {
		s.logger.Debugf("soc upload: bad signature: %v", err)
		s.logger.Error("soc upload: bad signature")
		jsonhttp.BadRequest(w, "bad signature")
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debugf("soc upload: read chunk data error: %v", err)
		s.logger.Error("soc upload: read chunk data error")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if len(data) < swarm.SpanSize {
		s.logger.Debugf("soc upload: chunk data too short")
		s.logger.Error("soc upload: %v", errBadRequestParams)
		jsonhttp.BadRequest(w, "short chunk data")
		return
	}

	if len(data) > swarm.ChunkSize+swarm.SpanSize {
		s.logger.Debugf("soc upload: chunk data exceeds %d bytes", swarm.ChunkSize+swarm.SpanSize)
		s.logger.Error("soc upload: chunk data error")
		jsonhttp.RequestEntityTooLarge(w, "payload too large")
		return
	}

	ch, err := cac.NewWithDataSpan(data)
	if err != nil {
		s.logger.Debugf("soc upload: create content addressed chunk: %v", err)
		s.logger.Error("soc upload: chunk data error")
		jsonhttp.BadRequest(w, "chunk data error")
		return
	}

	ss, err := soc.NewSigned(id, ch, owner, sig)
	if err != nil {
		s.logger.Debugf("soc upload: address soc error: %v", err)
		s.logger.Error("soc upload: address soc error")
		jsonhttp.Unauthorized(w, "invalid address")
		return
	}

	sch, err := ss.Chunk()
	if err != nil {
		s.logger.Debugf("soc upload: read chunk data error: %v", err)
		s.logger.Error("soc upload: read chunk data error")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if !soc.Valid(sch) {
		s.logger.Debugf("soc upload: invalid chunk: %v", err)
		s.logger.Error("soc upload: invalid chunk")
		jsonhttp.Unauthorized(w, "invalid chunk")
		return
	}

	ctx := r.Context()

	has, err := s.storer.Has(ctx, sch.Address())
	if err != nil {
		s.logger.Debugf("soc upload: store has: %v", err)
		s.logger.Error("soc upload: store has")
		jsonhttp.InternalServerError(w, "storage error")
		return
	}
	if has {
		s.logger.Error("soc upload: chunk already exists")
		jsonhttp.Conflict(w, "chunk already exists")
		return
	}
	batch, err := requestPostageBatchId(r)
	if err != nil {
		s.logger.Debugf("soc upload: postage batch id: %v", err)
		s.logger.Error("soc upload: postage batch id")
		jsonhttp.BadRequest(w, "invalid postage batch id")
		return
	}

	i, err := s.post.GetStampIssuer(batch)
	if err != nil {
		s.logger.Debugf("soc upload: postage batch issuer: %v", err)
		s.logger.Error("soc upload: postage batch issue")
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
	stamper := postage.NewStamper(i, s.signer)
	stamp, err := stamper.Stamp(sch.Address())
	if err != nil {
		s.logger.Debugf("soc upload: stamp: %v", err)
		s.logger.Error("soc upload: stamp error")
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
		s.logger.Debugf("soc upload: chunk write error: %v", err)
		s.logger.Error("soc upload: chunk write error")
		jsonhttp.BadRequest(w, "chunk write error")
		return
	}

	if strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true" {
		if err := s.pinning.CreatePin(ctx, sch.Address(), false); err != nil {
			s.logger.Debugf("soc upload: creation of pin for %q failed: %v", sch.Address(), err)
			s.logger.Error("soc upload: creation of pin failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
	}

	jsonhttp.Created(w, chunkAddressResponse{Reference: sch.Address()})
}

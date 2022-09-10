// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

type socPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

func (s *Service) socUploadHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Owner []byte `parse:"owner,addressToString" name:"owner" errMessage:"bad owner"`
		Id    []byte `parse:"id,addressToString" name:"id" errMessage:"bad id"`
		Sig   []byte `parse:"sig,addressToString" name:"signature" errMessage:"bad signature"`
	}{}

	if err := s.parseAndValidate(r, &path); err != nil {
		fmt.Printf("--parseAndValidate res %+v", path)
		s.logger.Debug("soc upload: parse owner string failed", "string", "", "error", err)
		s.logger.Error(nil, "soc upload: parse owner string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	fmt.Println("+++", path.Sig)
	//str := mux.Vars(r)["owner"]
	//owner, err := hex.DecodeString(str)
	//if err != nil {
	//	s.logger.Debug("soc upload: parse owner string failed", "string", str, "error", err)
	//	s.logger.Error(nil, "soc upload: parse owner string failed")
	//	jsonhttp.BadRequest(w, "bad owner")
	//	return
	//}
	//str = mux.Vars(r)["id"]
	//id, err := hex.DecodeString(mux.Vars(r)["id"])
	//if err != nil {
	//	s.logger.Debug("soc upload: parse id string failed", "string", str, "error", err)
	//	s.logger.Error(nil, "soc upload: parse id string failed")
	//	jsonhttp.BadRequest(w, "bad id")
	//	return
	//}
	//
	//sigStr := r.URL.Query().Get("sig")
	//if sigStr == "" {
	//	s.logger.Debug("soc upload: empty sig string")
	//	s.logger.Error(nil, "soc upload: empty sig string")
	//	jsonhttp.BadRequest(w, "empty signature")
	//	return
	//}
	//
	//sig, err := hex.DecodeString(sigStr)
	//if err != nil {
	//	s.logger.Debug("soc upload: decode sig string failed", "string", sigStr, "error", err)
	//	s.logger.Error(nil, "soc upload: decode sig string failed")
	//	jsonhttp.BadRequest(w, "bad signature")
	//	return
	//}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debug("soc upload: read body failed", "error", err)
		s.logger.Error(nil, "soc upload: read body failed")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if len(data) < swarm.SpanSize {
		s.logger.Debug("soc upload: chunk data too short")
		s.logger.Error(nil, "soc upload: chunk data too short")
		jsonhttp.BadRequest(w, "short chunk data")
		return
	}

	if len(data) > swarm.ChunkSize+swarm.SpanSize {
		s.logger.Debug("soc upload: chunk data exceeds required length", "required_length", swarm.ChunkSize+swarm.SpanSize)
		s.logger.Error(nil, "soc upload: chunk data exceeds required length")
		jsonhttp.RequestEntityTooLarge(w, "payload too large")
		return
	}

	ch, err := cac.NewWithDataSpan(data)
	if err != nil {
		s.logger.Debug("soc upload: create content addressed chunk failed", "error", err)
		s.logger.Error(nil, "soc upload: create content addressed chunk failed")
		jsonhttp.BadRequest(w, "chunk data error")
		return
	}

	ss, err := soc.NewSigned(path.Id, ch, path.Owner, path.Sig)
	if err != nil {
		s.logger.Debug("soc upload: create soc failed", "id", path.Id, "owner", path.Owner, "error", err)
		s.logger.Error(nil, "soc upload: create soc failed")
		jsonhttp.Unauthorized(w, "invalid address")
		return
	}

	sch, err := ss.Chunk()
	if err != nil {
		s.logger.Debug("soc upload: read chunk data failed", "error", err)
		s.logger.Error(nil, "soc upload: read chunk data failed")
		jsonhttp.InternalServerError(w, "cannot read chunk data")
		return
	}

	if !soc.Valid(sch) {
		s.logger.Debug("soc upload: invalid chunk", "error", err)
		s.logger.Error(nil, "soc upload: invalid chunk")
		jsonhttp.Unauthorized(w, "invalid chunk")
		return
	}

	ctx := r.Context()

	has, err := s.storer.Has(ctx, sch.Address())
	if err != nil {
		s.logger.Debug("soc upload: has check failed", "chunk_address", sch.Address(), "error", err)
		s.logger.Error(nil, "soc upload: has check failed")
		jsonhttp.InternalServerError(w, "storage error")
		return
	}
	if has {
		s.logger.Error(nil, "soc upload: chunk already exists")
		jsonhttp.Conflict(w, "chunk already exists")
		return
	}
	batch, err := requestPostageBatchId(r)
	if err != nil {
		s.logger.Debug("soc upload: parse postage batch id failed", "error", err)
		s.logger.Error(nil, "soc upload: parse postage batch id failed")
		jsonhttp.BadRequest(w, "invalid postage batch id")
		return
	}

	i, err := s.post.GetStampIssuer(batch)
	if err != nil {
		s.logger.Debug("soc upload: get postage batch issuer failed", "batch_id", fmt.Sprintf("%x", batch), "error", err)
		s.logger.Error(nil, "soc upload: get postage batch issue")
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
		s.logger.Debug("soc upload: stamp failed", "chunk_address", sch.Address(), "error", err)
		s.logger.Error(nil, "soc upload: stamp failed")
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
		s.logger.Debug("soc upload: write chunk failed", "chunk_address", sch.Address(), "error", err)
		s.logger.Error(nil, "soc upload: write chunk failed")
		jsonhttp.BadRequest(w, "chunk write error")
		return
	}

	if strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true" {
		if err := s.pinning.CreatePin(ctx, sch.Address(), false); err != nil {
			s.logger.Debug("soc upload: create pin failed", "chunk_address", sch.Address(), "error", err)
			s.logger.Error(nil, "soc upload: create pin failed")
			jsonhttp.InternalServerError(w, "soc upload: creation of pin failed")
			return
		}
	}

	jsonhttp.Created(w, chunkAddressResponse{Reference: sch.Address()})
}

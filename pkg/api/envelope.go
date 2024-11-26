// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

type postEnvelopeResponse struct {
	Issuer    string `json:"issuer"`    // Ethereum address of the postage batch owner
	Index     string `json:"index"`     // used index of the Postage Batch
	Timestamp string `json:"timestamp"` // timestamp of the postage stamp
	Signature string `json:"signature"` // postage stamp signature
}

// envelopePostHandler generates new postage stamp for requested chunk address
func (s *Service) envelopePostHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_envelope").Build()

	headers := struct {
		BatchID []byte `map:"Swarm-Postage-Batch-Id" validate:"required"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	paths := struct {
		Address swarm.Address `map:"address" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	stamper, save, err := s.getStamper(headers.BatchID)
	if err != nil {
		logger.Debug("get stamper failed", "error", err)
		logger.Error(err, "get stamper failed")
		switch {
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		default:
			jsonhttp.InternalServerError(w, nil)
		}
		return
	}

	stamp, err := stamper.Stamp(paths.Address, paths.Address)
	if err != nil {
		logger.Debug("split write all failed", "error", err)
		logger.Error(nil, "split write all failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "stamping failed")
		}
		return
	}
	err = save()
	if err != nil {
		jsonhttp.InternalServerError(w, "failed to save stamp issuer")
		return
	}

	issuer, err := s.signer.EthereumAddress()
	if err != nil {
		jsonhttp.InternalServerError(w, "signer ethereum address")
		return
	}
	jsonhttp.Created(w, postEnvelopeResponse{
		Issuer:    issuer.Hex(),
		Index:     hex.EncodeToString(stamp.Index()),
		Timestamp: hex.EncodeToString(stamp.Timestamp()),
		Signature: hex.EncodeToString(stamp.Sig()),
	})
}

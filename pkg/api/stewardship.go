// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/ethersphere/bee/pkg/log"
	"net/http"

	"github.com/ethersphere/bee/pkg/postage"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

// StewardshipPutHandler re-uploads root hash and all of its underlying associated chunks to the network.
func (s *Service) stewardshipPutHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("put_stewardship").Build()

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		BatchID []byte `map:"Swarm-Postage-Batch-Id"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	var (
		batchID []byte
		err     error
	)

	if len(headers.BatchID) == 0 {
		logger.Debug("missing postage batch id for re-upload")
		batchID, err = s.storer.BatchHint(paths.Address)
		if err != nil {
			logger.Debug("unable to find old batch for reference", log.LogItem{"error", err})
			logger.Error(nil, "unable to find old batch for reference")
			jsonhttp.NotFound(w, "unable to find old batch for reference, provide new batch id")
			return
		}
	} else {
		batchID = headers.BatchID
	}
	stamper, save, err := s.getStamper(batchID)
	if err != nil {
		switch {
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound) || errors.Is(err, storage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		default:
			jsonhttp.BadRequest(w, nil)
		}
		return
	}

	err = s.steward.Reupload(r.Context(), paths.Address, stamper)
	if err != nil {
		err = errors.Join(err, save())
		logger.Debug("re-upload failed", log.LogItem{"chunk_address", paths.Address}, log.LogItem{"error", err})
		logger.Error(nil, "re-upload failed")
		jsonhttp.InternalServerError(w, "re-upload failed")
		return
	}

	if err = save(); err != nil {
		logger.Debug("unable to save stamper data", log.LogItem{"batchID", batchID}, log.LogItem{"error", err})
		logger.Error(nil, "unable to save stamper data")
		jsonhttp.InternalServerError(w, "unable to save stamper data")
		return
	}

	jsonhttp.OK(w, nil)
}

type isRetrievableResponse struct {
	IsRetrievable bool `json:"isRetrievable"`
}

// stewardshipGetHandler checks whether the content on the given address is retrievable.
func (s *Service) stewardshipGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_stewardship").Build()

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	res, err := s.steward.IsRetrievable(r.Context(), paths.Address)
	if err != nil {
		logger.Debug("is retrievable check failed", log.LogItem{"chunk_address", paths.Address}, log.LogItem{"error", err})
		logger.Error(nil, "is retrievable")
		jsonhttp.InternalServerError(w, "is retrievable check failed")
		return
	}
	jsonhttp.OK(w, isRetrievableResponse{
		IsRetrievable: res,
	})
}

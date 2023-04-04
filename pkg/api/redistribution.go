// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/tracing"
)

type redistributionStatusResponse struct {
	HasSufficientFunds bool           `json:"hasSufficientFunds"`
	IsFrozen           bool           `json:"isFrozen"`
	IsFullySynced      bool           `json:"isFullySynced"`
	Phase              string         `json:"phase"`
	Round              uint64         `json:"round"`
	LastWonRound       uint64         `json:"lastWonRound"`
	LastPlayedRound    uint64         `json:"lastPlayedRound"`
	LastFrozenRound    uint64         `json:"lastFrozenRound"`
	Block              uint64         `json:"block"`
	Reward             *bigint.BigInt `json:"reward"`
	Fees               *bigint.BigInt `json:"fees"`
}

func (s *Service) redistributionStatusHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_redistributionstate").Build())

	if s.beeMode != FullMode {
		jsonhttp.BadRequest(w, errOperationSupportedOnlyInFullMode)
		return
	}

	status, err := s.redistributionAgent.Status()
	if err != nil {
		logger.Debug("get redistribution status", "overlay_address", s.overlay.String(), "error", err)
		logger.Error(nil, "get redistribution status")
		jsonhttp.InternalServerError(w, "failed to get redistribution status")
		return
	}

	hasSufficientFunds, err := s.redistributionAgent.HasEnoughFundsToPlay(r.Context())
	if err != nil {
		logger.Debug("has enough funds to play", "overlay_address", s.overlay.String(), "error", err)
		logger.Error(nil, "has enough funds to play")
		jsonhttp.InternalServerError(w, "failed to calculate if node has enough funds to play")
		return
	}

	jsonhttp.OK(w, redistributionStatusResponse{
		HasSufficientFunds: hasSufficientFunds,
		IsFrozen:           status.IsFrozen,
		IsFullySynced:      status.IsFullySynced,
		Phase:              status.Phase.String(),
		LastWonRound:       status.LastWonRound,
		LastPlayedRound:    status.LastPlayedRound,
		LastFrozenRound:    status.LastFrozenRound,
		Round:              status.Round,
		Block:              status.Block,
		Reward:             bigint.Wrap(status.Reward),
		Fees:               bigint.Wrap(status.Fees),
	})
}

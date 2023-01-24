// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/tracing"
	"net/http"
)

type nodeStatusResponse struct {
	State        string         `json:"state"`
	Phase        string         `json:"phase"`
	Round        uint64         `json:"round"`
	LastWonRound uint64         `json:"lastWonRound"`
	Block        uint64         `json:"block"`
	Reward       *bigint.BigInt `json:"reward"`
	Fees         *bigint.BigInt `json:"fees"`
}

func (s *Service) redistributionStatusHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_redistributionstate").Build())

	status, err := s.redistributionAgent.Status()
	if err != nil {
		logger.Debug("get redistribution status", "overlay address", s.overlay.String(), "error", err)
		logger.Error(nil, "get redistribution status")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	jsonhttp.OK(w, nodeStatusResponse{
		State:        status.State.String(),
		Phase:        status.Phase.String(),
		LastWonRound: status.LastWonRound,
		Round:        status.Round,
		Block:        status.Block,
		Reward:       bigint.Wrap(status.Reward),
		Fees:         bigint.Wrap(status.Fees),
	})
}

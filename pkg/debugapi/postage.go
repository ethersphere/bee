// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type reserveStateResponse struct {
	Radius        uint8          `json:"radius"`
	StorageRadius uint8          `json:"storageRadius"`
	Available     int64          `json:"available"`
	Outer         *bigint.BigInt `json:"outer"` // lower value limit for outer layer = the further half of chunks
	Inner         *bigint.BigInt `json:"inner"`
}

type chainStateResponse struct {
	Block        uint64         `json:"block"`        // The block number of the last postage event.
	TotalAmount  *bigint.BigInt `json:"totalAmount"`  // Cumulative amount paid per stamp.
	CurrentPrice *bigint.BigInt `json:"currentPrice"` // Bzz/chunk/block normalised price.
}

func (s *Service) reserveStateHandler(w http.ResponseWriter, _ *http.Request) {
	state := s.batchStore.GetReserveState()

	jsonhttp.OK(w, reserveStateResponse{
		Radius:    state.Radius,
		Available: state.Available,
		Outer:     bigint.Wrap(state.Outer),
		Inner:     bigint.Wrap(state.Inner),
	})
}

// chainStateHandler returns the current chain state.
func (s *Service) chainStateHandler(w http.ResponseWriter, _ *http.Request) {
	state := s.batchStore.GetChainState()

	jsonhttp.OK(w, chainStateResponse{
		Block:        state.Block,
		TotalAmount:  bigint.Wrap(state.TotalAmount),
		CurrentPrice: bigint.Wrap(state.CurrentPrice),
	})
}

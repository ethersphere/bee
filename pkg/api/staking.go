// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"fmt"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storageincentives/staking"
	"github.com/gorilla/mux"
)

func (s *Service) stakingAccessHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.stakingSem.TryAcquire(1) {
			s.logger.Debug("staking access: simultaneous on-chain operations not supported")
			s.logger.Error(nil, "staking access: simultaneous on-chain operations not supported")
			jsonhttp.TooManyRequests(w, "simultaneous on-chain operations not supported")
			return
		}
		defer s.stakingSem.Release(1)

		h.ServeHTTP(w, r)
	})
}

type getStakeResponse struct {
	StakedAmount *big.Int `json:"stakedAmount"`
}

func (s *Service) stakingDepositHandler(w http.ResponseWriter, r *http.Request) {
	stakedAmount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error(nil, "deposit stake: invalid amount")
		jsonhttp.BadRequest(w, "invalid staking amount")
		return
	}
	err := s.stakingContract.DepositStake(r.Context(), stakedAmount)
	if err != nil {
		if errors.Is(err, staking.ErrInsufficientStakeAmount) {
			s.logger.Debug("deposit stake: minimum BZZ required for staking", "minimum_stake", staking.MinimumStakeAmount, "error", err)
			s.logger.Error(nil, fmt.Sprintf("deposit stake: minimum %d BZZ required for staking", staking.MinimumStakeAmount.Int64()))
			jsonhttp.BadRequest(w, fmt.Sprintf("minimum %d BZZ required for staking", staking.MinimumStakeAmount.Int64()))
			return
		}
		if errors.Is(err, staking.ErrNotImplemented) {
			s.logger.Debug("deposit stake: not implemented", "error", err)
			s.logger.Error(nil, "deposit stake: not implemented")
			jsonhttp.NotImplemented(w, "not implemented")
			return
		}
		if errors.Is(err, staking.ErrInsufficientFunds) {
			s.logger.Debug("deposit stake: out of funds", "error", err)
			s.logger.Error(nil, "deposit stake: out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		s.logger.Debug("deposit stake: deposit failed", "error", err)
		s.logger.Error(nil, "deposit stake: deposit failed")
		jsonhttp.InternalServerError(w, "cannot stake")
		return
	}
	jsonhttp.OK(w, nil)
}

func (s *Service) getStakedAmountHandler(w http.ResponseWriter, r *http.Request) {
	stakedAmount, err := s.stakingContract.GetStake(r.Context())
	if err != nil {
		s.logger.Debug("get stake: get staked amount failed", "overlayAddr", s.overlay, "error", err)
		s.logger.Error(nil, "get stake: get staked amount failed")
		jsonhttp.InternalServerError(w, "get staked amount failed")
		return
	}

	jsonhttp.OK(w, getStakeResponse{StakedAmount: stakedAmount})
}

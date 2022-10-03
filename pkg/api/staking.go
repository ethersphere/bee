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
	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
	"github.com/ethersphere/bee/pkg/swarm"
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
	overlayAddr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debug("deposit stake: decode overlay address failed", "overlay", overlayAddr, "error", err)
		s.logger.Error(nil, "deposit stake: decode overlay address failed")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	stakedAmount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error(nil, "deposit stake: invalid amount")
		jsonhttp.BadRequest(w, "invalid staking amount")
		return
	}
	err = s.stakingContract.DepositStake(r.Context(), stakedAmount, overlayAddr)
	if err != nil {
		if errors.Is(err, stakingcontract.ErrInsufficientStakeAmount) {
			s.logger.Debug("deposit stake: minimum BZZ required for staking", "minimum_stake", stakingcontract.MinimumStakeAmount, "error", err)
			s.logger.Error(nil, fmt.Sprintf("deposit stake: minimum %d BZZ required for staking", stakingcontract.MinimumStakeAmount.Int64()))
			jsonhttp.BadRequest(w, "minimum 100000000000000000 BZZ required for staking")
			return
		}
		if errors.Is(err, stakingcontract.ErrNotImplemented) {
			s.logger.Debug("deposit stake: not implemented", "error", err)
			s.logger.Error(nil, "deposit stake: not implemented")
			jsonhttp.NotImplemented(w, "not implemented")
			return
		}
		if errors.Is(err, stakingcontract.ErrInsufficientFunds) {
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
	overlayAddr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debug("get stake: decode overlay address failed", "overlay", overlayAddr, "error", err)
		s.logger.Error(nil, "get stake: decode overlay address failed")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	stakedAmount, err := s.stakingContract.GetStake(r.Context(), overlayAddr)
	if err != nil {
		s.logger.Debug("get stake: get staked amount failed", "overlayAddr", overlayAddr, "error", err)
		s.logger.Error(nil, "get stake: get staked amount failed")
		jsonhttp.InternalServerError(w, "get staked amount failed")
		return
	}

	jsonhttp.OK(w, getStakeResponse{StakedAmount: stakedAmount})
}

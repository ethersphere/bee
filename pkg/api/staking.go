// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/sctx"
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
	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error(nil, "deposit stake: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	if limit, ok := r.Header[gasLimitHeader]; ok {
		l, err := strconv.ParseUint(limit[0], 10, 64)
		if err != nil {
			s.logger.Error(err, "deposit stake: bad gas limit")
			jsonhttp.BadRequest(w, errBadGasLimit)
			return
		}
		ctx = sctx.SetGasLimit(ctx, l)
	}

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
	err = s.stakingContract.DepositStake(ctx, *stakedAmount, overlayAddr)
	if err != nil {
		if errors.Is(err, stakingcontract.ErrInsufficentStakeAmount) {
			s.logger.Debug("deposit stake: insufficient stake amount", "error", err)
			s.logger.Error(nil, "deposit stake: insufficient stake amount")
			jsonhttp.BadRequest(w, "minimum 1 BZZ required for staking")
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
	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error(nil, "get stake: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	if limit, ok := r.Header[gasLimitHeader]; ok {
		l, err := strconv.ParseUint(limit[0], 10, 64)
		if err != nil {
			s.logger.Error(err, "get stake: bad gas limit")
			jsonhttp.BadRequest(w, errBadGasLimit)
			return
		}
		ctx = sctx.SetGasLimit(ctx, l)
	}

	overlayAddr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debug("get stake: decode overlay address failed", "overlay", overlayAddr, "error", err)
		s.logger.Error(nil, "get stake: decode overlay address failed")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	stakedAmount, err := s.stakingContract.GetStake(ctx, overlayAddr)
	if err != nil {
		s.logger.Debug("get stake: get staked amount failed", "overlayAddr", overlayAddr, "error", err)
		s.logger.Error(nil, "get stake: get staked amount failed")
		jsonhttp.InternalServerError(w, "get staked amount failed")
		return
	}

	jsonhttp.OK(w, getStakeResponse{StakedAmount: &stakedAmount})
}

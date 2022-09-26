// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"errors"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
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

func (s *Service) stakingDepositHandler(w http.ResponseWriter, r *http.Request) {
	overlayAddr, err := hex.DecodeString(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debug("get stake: decode overlay address failed", "string", overlayAddr, "error", err)
		s.logger.Error(nil, "get stake: decode overlay address failed")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}
	if len(overlayAddr) == 0 {
		overlayAddr = s.overlay.Bytes()
	}

	stakedAmount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error(nil, "deposit stake: invalid amount")
		jsonhttp.BadRequest(w, "invalid staking amount")
		return
	}
	err = s.stakingContract.DepositStake(r.Context(), stakedAmount, overlayAddr)
	if err != nil {
		if errors.Is(err, stakingcontract.ErrInvalidStakeAmount) {
			s.logger.Debug("deposit stake: invalid stake amount", "error", err)
			s.logger.Error(nil, "deposit stake: invalid stake amount")
			jsonhttp.BadRequest(w, "minimum 1 BZZ required for staking")
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

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/bigint"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
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
	StakedAmount *bigint.BigInt `json:"stakedAmount"`
}

type getWithdrawableResponse struct {
	WithdrawableAmount *bigint.BigInt `json:"withdrawableAmount"`
}
type stakeTransactionReponse struct {
	TxHash string `json:"txHash"`
}

func (s *Service) stakingDepositHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_stake_deposit").Build()

	paths := struct {
		Amount *big.Int `map:"amount" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	txHash, err := s.stakingContract.DepositStake(r.Context(), paths.Amount)
	if err != nil {
		if errors.Is(err, staking.ErrInsufficientStakeAmount) {
			logger.Debug("insufficient stake amount", "minimum_stake", staking.MinimumStakeAmount, "error", err)
			logger.Error(nil, "insufficient stake amount")
			jsonhttp.BadRequest(w, "insufficient stake amount")
			return
		}
		if errors.Is(err, staking.ErrNotImplemented) {
			logger.Debug("not implemented", "error", err)
			logger.Error(nil, "not implemented")
			jsonhttp.NotImplemented(w, "not implemented")
			return
		}
		if errors.Is(err, staking.ErrInsufficientFunds) {
			logger.Debug("out of funds", "error", err)
			logger.Error(nil, "out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		logger.Debug("deposit failed", "error", err)
		logger.Error(nil, "deposit failed")
		jsonhttp.InternalServerError(w, "cannot stake")
		return
	}
	jsonhttp.OK(w, stakeTransactionReponse{
		TxHash: txHash.String(),
	})
}

func (s *Service) getPotentialStake(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_stake").Build()

	stakedAmount, err := s.stakingContract.GetPotentialStake(r.Context())
	if err != nil {
		logger.Debug("get staked amount failed", "overlayAddr", s.overlay, "error", err)
		logger.Error(nil, "get staked amount failed")
		jsonhttp.InternalServerError(w, "get staked amount failed")
		return
	}

	jsonhttp.OK(w, getStakeResponse{StakedAmount: bigint.Wrap(stakedAmount)})
}

func (s *Service) getWithdrawableStakeHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_stake").Build()

	withdrawableAmount, err := s.stakingContract.GetWithdrawableStake(r.Context())
	if err != nil {
		logger.Debug("get staked amount failed", "overlayAddr", s.overlay, "error", err)
		logger.Error(nil, "get staked amount failed")
		jsonhttp.InternalServerError(w, "get staked amount failed")
		return
	}

	jsonhttp.OK(w, getWithdrawableResponse{WithdrawableAmount: bigint.Wrap(withdrawableAmount)})
}

func (s *Service) withdrawStakeHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("withdraw_stake").Build()

	txHash, err := s.stakingContract.WithdrawStake(r.Context())
	if err != nil {
		if errors.Is(err, staking.ErrInsufficientStake) {
			logger.Debug("insufficient stake", "overlayAddr", s.overlay, "error", err)
			logger.Error(nil, "insufficient stake")
			jsonhttp.BadRequest(w, "insufficient stake to withdraw")
			return
		}
		logger.Debug("withdraw stake failed", "error", err)
		logger.Error(nil, "withdraw stake failed")
		jsonhttp.InternalServerError(w, "cannot withdraw stake")
		return
	}

	jsonhttp.OK(w, stakeTransactionReponse{TxHash: txHash.String()})
}

func (s *Service) migrateStakeHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("migrate_stake").Build()

	txHash, err := s.stakingContract.MigrateStake(r.Context())
	if err != nil {
		if errors.Is(err, staking.ErrInsufficientStake) {
			logger.Debug("insufficient stake", "overlayAddr", s.overlay, "error", err)
			logger.Error(nil, "insufficient stake")
			jsonhttp.BadRequest(w, "insufficient stake to migrate")
			return
		}
		if errors.Is(err, staking.ErrNotPaused) {
			logger.Debug("contract is not paused", "error", err)
			logger.Error(nil, "contract is not paused")
			jsonhttp.BadRequest(w, "contract is not paused")
			return
		}
		logger.Debug("migrate stake failed", "error", err)
		logger.Error(nil, "migrate stake failed")
		jsonhttp.InternalServerError(w, "cannot migrate stake")
		return
	}

	jsonhttp.OK(w, stakeTransactionReponse{TxHash: txHash.String()})
}

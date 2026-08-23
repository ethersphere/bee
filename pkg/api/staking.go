// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
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
	StakedAmount   *bigint.BigInt `json:"stakedAmount"`
	MinimumDeposit *bigint.BigInt `json:"minimumDeposit"`
}

type getWithdrawableResponse struct {
	WithdrawableAmount *bigint.BigInt `json:"withdrawableAmount"`
}
type stakeTransactionReponse struct {
	TxHash string `json:"txHash"`
}

type stakeDepositErrorResponse struct {
	Code           int            `json:"code"`
	Message        string         `json:"message"`
	MinimumDeposit *bigint.BigInt `json:"minimumDeposit"`
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
		var minErr *staking.MinDepositError
		if errors.As(err, &minErr) {
			logger.Debug("insufficient stake amount", "minimum_deposit", minErr.Minimum, "error", err)
			logger.Error(nil, "insufficient stake amount")
			jsonhttp.BadRequest(w, stakeDepositErrorResponse{
				Code:           http.StatusBadRequest,
				Message:        "insufficient stake amount",
				MinimumDeposit: bigint.Wrap(minErr.Minimum),
			})
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

	// if the deposit is successful, we should update the height of the node in the staking contract
	// this is done to make sure that the node is participating in the redistribution game with the correct height
	// if the node has started with insufficient stake
	tx, updated, err := s.stakingContract.UpdateHeight(r.Context())
	if err != nil {
		logger.Debug("update height failed", "error", err)
		logger.Error(nil, "update height failed")
	} else if updated {
		logger.Warning("reserve capacity doubling updated after stake deposit. Node will be FROZEN for ~2 rounds.", "transaction", tx)
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

	minDeposit, err := s.stakingContract.GetMinDeposit(r.Context())
	if err != nil {
		logger.Debug("get minimum deposit failed", "overlayAddr", s.overlay, "error", err)
		logger.Error(nil, "get minimum deposit failed")
		jsonhttp.InternalServerError(w, "get minimum deposit failed")
		return
	}

	jsonhttp.OK(w, getStakeResponse{
		StakedAmount:   bigint.Wrap(stakedAmount),
		MinimumDeposit: bigint.Wrap(minDeposit),
	})
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

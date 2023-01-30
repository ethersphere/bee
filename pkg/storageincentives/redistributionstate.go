// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/transaction"
)

const loggerNameNode = "nodestatus"

const (
	redistributionStatusKey = "redistribution_state"
	saveStatusInterval      = time.Second
)

type RedistributionState struct {
	mtx sync.Mutex

	stateStore     storage.StateStorer
	erc20Service   erc20.Service
	logger         log.Logger
	ethAddress     common.Address
	status         *Status
	currentBalance *big.Int
	txService      transaction.Service
}

// Status provide internal status of the nodes in the redistribution game
type Status struct {
	Phase           PhaseType
	IsFrozen        bool
	Round           uint64
	LastWonRound    uint64
	LastPlayedRound uint64
	LastFrozenRound uint64
	Block           uint64
	Reward          *big.Int
	Fees            *big.Int
}

func NewRedistributionState(logger log.Logger, ethAddress common.Address, stateStore storage.StateStorer, erc20Service erc20.Service, contract transaction.Service) (*RedistributionState, error) {
	s := &RedistributionState{
		ethAddress:     ethAddress,
		stateStore:     stateStore,
		erc20Service:   erc20Service,
		logger:         logger.WithName(loggerNameNode).Register(),
		currentBalance: big.NewInt(0),
		txService:      contract,
		status: &Status{
			Reward: big.NewInt(0),
			Fees:   big.NewInt(0),
		},
	}

	status, err := s.Status()
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.status = &Status{
				Reward: big.NewInt(0),
				Fees:   big.NewInt(0),
			}
			return s, nil
		}
		return nil, err
	}

	s.status = status
	return s, nil
}

func (r *RedistributionState) SetCurrentEvent(p PhaseType, round uint64, b uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.Phase = p
	r.status.Round = round
	r.status.Block = b
	r.save()
}

func (r *RedistributionState) SetFrozen(f bool, round uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.IsFrozen = f
	if f {
		r.status.LastFrozenRound = round
	}
	r.save()
}

func (r *RedistributionState) SetLastWonRound(round uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.LastWonRound = round
	r.save()
}

func (r *RedistributionState) SetLastPlayedRound(p uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.LastPlayedRound = p
	r.save()
}

// AddFee sets the internal node status
func (r *RedistributionState) AddFee(ctx context.Context, txHash common.Hash) {

	fee, err := r.txService.TransactionFee(ctx, txHash)
	if err != nil {
		return
	}
	r.mtx.Lock()
	r.status.Fees.Add(r.status.Fees, fee)
	r.save()
	r.mtx.Unlock()
}

// CalculateWinnerReward calculates the reward for the winner
func (r *RedistributionState) CalculateWinnerReward(ctx context.Context) error {
	currentBalance, err := r.erc20Service.BalanceOf(ctx, r.ethAddress)
	if err != nil {
		r.logger.Debug("error getting balance", "error", err)
		return err
	}
	if currentBalance != nil {
		r.mtx.Lock()
		r.status.Reward.Add(r.status.Reward, currentBalance.Sub(currentBalance, r.currentBalance))
		r.save()
		r.mtx.Unlock()
	}
	return nil
}

// Status returns the node status
func (r *RedistributionState) Status() (*Status, error) {
	status := new(Status)
	if err := r.stateStore.Get(redistributionStatusKey, status); err != nil {
		return nil, err
	}
	return status, nil
}

func (r *RedistributionState) SetBalance(ctx context.Context) error {
	// get current balance
	currentBalance, err := r.erc20Service.BalanceOf(ctx, r.ethAddress)
	if err != nil {
		r.logger.Debug("error getting balance", "error", err)
		return err
	}
	r.mtx.Lock()
	r.currentBalance.Set(currentBalance)
	r.mtx.Unlock()
	return nil
}

func (r *RedistributionState) save() {
	err := r.stateStore.Put(redistributionStatusKey, r.status)
	if err != nil {
		r.logger.Error(err, "saving redistribution status")
	}
}

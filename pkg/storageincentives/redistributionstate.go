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
	IsFullySynced   bool
	Round           uint64
	LastWonRound    uint64
	LastPlayedRound uint64
	LastFrozenRound uint64
	Block           uint64
	Reward          *big.Int
	Fees            *big.Int
	TxCount         uint64
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

func (r *RedistributionState) SetCurrentEvent(phase PhaseType, round uint64, block uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.Phase = phase
	r.status.Round = round
	r.status.Block = block
	r.save()
}

func (r *RedistributionState) SetFrozen(isFrozen bool, round uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if isFrozen && !r.status.IsFrozen { // record fronzen round if not set already
		r.status.LastFrozenRound = round
	}
	r.status.IsFrozen = isFrozen
	r.save()
}

func (r *RedistributionState) SetLastWonRound(round uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.LastWonRound = round
	r.save()
}

func (r *RedistributionState) IsFullySynced(isSynced bool) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.IsFullySynced = isSynced
	r.save()
}

func (r *RedistributionState) SetLastPlayedRound(round uint64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.status.LastPlayedRound = round
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
	r.status.TxCount++
	r.save()
	r.mtx.Unlock()
}

func (r *RedistributionState) AvgFee() *big.Int {
	avgFee := big.NewInt(0).Set(r.status.Fees)
	avgFee.Div(avgFee, big.NewInt(int64(r.status.TxCount)))

	return avgFee
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

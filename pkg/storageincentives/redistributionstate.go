// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
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

func NewRedistributionState(logger log.Logger, ethAddress common.Address, stateStore storage.StateStorer, erc20Service erc20.Service, contract transaction.Service) *RedistributionState {
	return &RedistributionState{
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
}

func (n *RedistributionState) SetCurrentEvent(p PhaseType, r uint64, b uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.Phase = p
	n.status.Round = r
	n.status.Block = b
	n.save()
}

func (n *RedistributionState) SetFrozen(f bool, r uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.IsFrozen = f
	if f {
		n.status.LastFrozenRound = r
	}
	n.save()
}

func (n *RedistributionState) SetLastWonRound(r uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastWonRound = r
	n.save()
}

func (n *RedistributionState) SetLastPlayedRound(p uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastPlayedRound = p
	n.save()
}

// AddFee sets the internal node status
func (n *RedistributionState) AddFee(ctx context.Context, txHash common.Hash) {

	fee, err := n.txService.TransactionFee(ctx, txHash)
	if err != nil {
		return
	}
	n.mtx.Lock()
	n.status.Fees.Add(n.status.Fees, fee)
	n.save()
	n.mtx.Unlock()
}

// CalculateWinnerReward calculates the reward for the winner
func (n *RedistributionState) CalculateWinnerReward(ctx context.Context) error {
	currentBalance, err := n.erc20Service.BalanceOf(ctx, n.ethAddress)
	if err != nil {
		n.logger.Debug("error getting balance", "error", err)
		return err
	}
	if currentBalance != nil {
		n.mtx.Lock()
		n.status.Reward.Add(n.status.Reward, currentBalance.Sub(currentBalance, n.currentBalance))
		n.save()
		n.mtx.Unlock()
	}
	return nil
}

// Status returns the node status
func (n *RedistributionState) Status() (*Status, error) {
	status := new(Status)
	if err := n.stateStore.Get(redistributionStatusKey, status); err != nil {
		return nil, err
	}
	return status, nil
}

func (n *RedistributionState) SetBalance(ctx context.Context) error {
	// get current balance
	currentBalance, err := n.erc20Service.BalanceOf(ctx, n.ethAddress)
	if err != nil {
		n.logger.Debug("error getting balance", "error", err)
		return err
	}
	n.mtx.Lock()
	n.currentBalance.Set(currentBalance)
	n.mtx.Unlock()
	return nil
}

func (n *RedistributionState) save() {
	err := n.stateStore.Put(redistributionStatusKey, n.status)
	if err != nil {
		n.logger.Error(err, "savıng redıstrubutıon status")
	}
}

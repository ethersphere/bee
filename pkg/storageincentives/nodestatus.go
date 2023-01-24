// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"math/big"
	"sync"
)

const redistributionStatusKey = "redistribution_state"
const loggerNameNode = "nodestatus"

type RedistributionState struct {
	stateStore     storage.StateStorer
	erc20Service   erc20.Service
	logger         log.Logger
	overlay        swarm.Address
	mtx            sync.Mutex
	status         Status
	currentBalance *big.Int
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

func NewRedistributionState(logger log.Logger, stateStore storage.StateStorer, erc20Service erc20.Service) RedistributionState {
	return RedistributionState{
		stateStore:     stateStore,
		erc20Service:   erc20Service,
		logger:         logger.WithName(loggerNameNode).Register(),
		currentBalance: big.NewInt(0),
		status: Status{
			Round:  0,
			Block:  0,
			Reward: nil,
			Fees:   nil,
		},
	}
}

func (n *RedistributionState) setCurrentEvent(p PhaseType, r uint64, b uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.Phase = p
	n.status.Round = r
	n.status.Block = b
	n.saveStatus()
}

func (n *RedistributionState) isFrozen(f bool) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.IsFrozen = f
	n.saveStatus()
}

func (n *RedistributionState) setLastWonRound(r uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastWonRound = r
	n.saveStatus()
}

func (n *RedistributionState) setLastFrozenRound(f uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastFrozenRound = f
	n.saveStatus()
}

func (n *RedistributionState) setLastPlayedRound(p uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastPlayedRound = p
	n.saveStatus()
}

// AddFee sets the internal node status
func (n *RedistributionState) AddFee(fee *big.Int) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	if fee != nil {
		n.status.Fees.Add(n.status.Fees, fee)
		n.saveStatus()
	}
}

// CalculateWinnerReward calculates the reward for the winner
func (n *RedistributionState) CalculateWinnerReward(ctx context.Context) error {
	currentBalance, err := n.erc20Service.BalanceOf(ctx, common.HexToAddress(n.overlay.String()))
	if err != nil {
		n.logger.Debug("error getting balance", "error", err, "overly address", n.overlay.String())
		return err
	}
	if currentBalance != nil {
		n.status.Reward.Add(n.status.Reward, currentBalance.Sub(currentBalance, n.currentBalance))
	}
	return nil
}

// saveStatus saves the node status to the database
func (n *RedistributionState) saveStatus() {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	err := n.stateStore.Put(redistributionStatusKey, n.status)
	if err != nil {
		n.logger.Error(err, "error saving node status")
		return
	}
}

// Status returns the node status
func (n *RedistributionState) Status() (*Status, error) {
	status := new(Status)
	if err := n.stateStore.Get(redistributionStatusKey, status); err != nil {
		return nil, err
	}
	return status, nil
}

func (n *RedistributionState) setBalance(ctx context.Context) error {
	// get current balance
	currentBalance, err := n.erc20Service.BalanceOf(ctx, common.HexToAddress(n.overlay.String()))
	if err != nil {
		n.logger.Debug("error getting balance", "error", err, "overly address", n.overlay.String())
		return err
	}
	n.currentBalance.Set(currentBalance)
	return nil
}

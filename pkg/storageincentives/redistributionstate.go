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
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
)

const loggerNameNode = "nodestatus"

const (
	redistributionStatusKey = "redistribution_state"
	saveStatusInterval      = time.Second
)

type RedistributionState struct {
	stateStore     storage.StateStorer
	erc20Service   erc20.Service
	logger         log.Logger
	overlay        swarm.Address
	mtx            sync.Mutex
	status         Status
	currentBalance *big.Int
	txService      transaction.Service
	saveStatusC    chan Status
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

func NewRedistributionState(logger log.Logger, stateStore storage.StateStorer, erc20Service erc20.Service, contract transaction.Service) RedistributionState {
	return RedistributionState{
		stateStore:     stateStore,
		erc20Service:   erc20Service,
		logger:         logger.WithName(loggerNameNode).Register(),
		currentBalance: big.NewInt(0),
		txService:      contract,
		status: Status{
			Round:  0,
			Block:  0,
			Reward: nil,
			Fees:   big.NewInt(0),
		},
		saveStatusC: make(chan Status, 8),
	}
}

func (n *RedistributionState) Stop() {
	close(n.saveStatusC)
}

func (n *RedistributionState) Start() {
	go n.saveStateHandler()
}

func (n *RedistributionState) saveStateHandler() {
	var status *Status // when nil latests is saved, when not nil status is dirty

	saveStatus := func() {
		if status == nil {
			return
		}

		err := n.stateStore.Put(redistributionStatusKey, *status)
		if err != nil {
			n.logger.Error(err, "error saving node status")
		} else {
			status = nil
		}
	}

	// save status is called in deffer to ensure that
	// state is stored before handler exists
	defer saveStatus()

	saveTicker := time.NewTicker(saveStatusInterval)
	defer saveTicker.Stop()

	for {
		select {
		case newStatus, ok := <-n.saveStatusC:
			if !ok {
				return
			}

			status = &newStatus

		case <-saveTicker.C:
			saveStatus()
		}
	}
}

func (n *RedistributionState) SetCurrentEvent(p PhaseType, r uint64, b uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.Phase = p
	n.status.Round = r
	n.status.Block = b

	n.saveStatusC <- n.status
}

func (n *RedistributionState) SetFrozen(f bool, r uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.IsFrozen = f
	if f {
		n.status.LastFrozenRound = r
	}

	n.saveStatusC <- n.status
}

func (n *RedistributionState) SetLastWonRound(r uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastWonRound = r

	n.saveStatusC <- n.status
}

func (n *RedistributionState) SetLastPlayedRound(p uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastPlayedRound = p

	n.saveStatusC <- n.status
}

// AddFee sets the internal node status
func (n *RedistributionState) AddFee(ctx context.Context, txHash common.Hash) {
	fee, err := n.txService.TransactionFee(ctx, txHash)
	if err != nil {
		return
	}

	n.mtx.Lock()
	n.status.Fees.Add(n.status.Fees, fee)
	n.saveStatusC <- n.status
	n.mtx.Unlock()
}

// CalculateWinnerReward calculates the reward for the winner
func (n *RedistributionState) CalculateWinnerReward(ctx context.Context) error {
	currentBalance, err := n.erc20Service.BalanceOf(ctx, common.HexToAddress(n.overlay.String()))
	if err != nil {
		n.logger.Debug("error getting balance", "error", err, "overly address", n.overlay.String())
		return err
	}
	if currentBalance != nil {
		n.mtx.Lock()
		n.status.Reward.Add(n.status.Reward, currentBalance.Sub(currentBalance, n.currentBalance))
		n.saveStatusC <- n.status
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
	currentBalance, err := n.erc20Service.BalanceOf(ctx, common.HexToAddress(n.overlay.String()))
	if err != nil {
		n.logger.Debug("error getting balance", "error", err, "overly address", n.overlay.String())
		return err
	}
	n.mtx.Lock()
	n.currentBalance.Set(currentBalance)
	n.mtx.Unlock()
	return nil
}

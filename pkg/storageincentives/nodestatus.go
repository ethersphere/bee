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

const (
	redistributionStatusKey = "redistribution_state_"
)

type NodeState struct {
	stateStore     storage.StateStorer
	erc20Service   erc20.Service
	logger         log.Logger
	overlay        swarm.Address
	mtx            sync.Mutex
	nodeStatus     NodeStatus
	initialBalance *big.Int
}

// NodeStatus provide internal status of the nodes in the redistribution game
type NodeStatus struct {
	State  PhaseType `json:"state"`
	Round  uint64    `json:"round"`
	Block  uint64    `json:"block"`
	Reward *big.Int  `json:"reward"`
	Fees   *big.Int  `json:"fees"`
}

func NewNode(logger log.Logger, stateStore storage.StateStorer, erc20Service erc20.Service) NodeState {
	return NodeState{
		stateStore:     stateStore,
		erc20Service:   erc20Service,
		logger:         logger.WithName("nodestatus").Register(),
		initialBalance: big.NewInt(0),
		nodeStatus: NodeStatus{
			Round:  0,
			Block:  0,
			Reward: nil,
			Fees:   nil,
		},
	}
}

func (n *NodeState) SetPhase(p PhaseType) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.nodeStatus.State = p
}

func (n *NodeState) SetRound(r uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.nodeStatus.Round = r
}

func (n *NodeState) SetBlock(b uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.nodeStatus.Block = b
}

// SetFee sets the internal node status
func (n *NodeState) SetFee(fee *big.Int) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	if fee != nil {
		n.nodeStatus.Fees = n.nodeStatus.Fees.Add(n.nodeStatus.Fees, fee)
	}
}

// CalculateWinnerReward calculates the reward for the winner
func (n *NodeState) CalculateWinnerReward() error {
	currentBalance, err := n.erc20Service.BalanceOf(context.Background(), common.HexToAddress(n.overlay.String()))
	if err != nil {
		n.logger.Debug("error getting balance", "error", err, "overly address", n.overlay.String())
		return err
	}
	if currentBalance != nil {
		n.nodeStatus.Reward = currentBalance.Sub(currentBalance, n.initialBalance)
	}
	return nil
}

// SaveStatus saves the node status to the database
func (n *NodeState) SaveStatus() {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	err := n.stateStore.Put(redistributionStatusKey, n.nodeStatus)
	if err != nil {
		n.logger.Error(err, "error saving node status")
		return
	}
}

// Status returns the node status
func (n *NodeState) Status() (status NodeStatus, err error) {
	return status, n.stateStore.Get(redistributionStatusKey, &status)
}

func (n *NodeState) SetBalance() error {
	// get current balance
	currentBalance, err := n.erc20Service.BalanceOf(context.Background(), common.HexToAddress(n.overlay.String()))
	if err != nil {
		n.logger.Debug("error getting balance", "error", err, "overly address", n.overlay.String())
		return err
	}
	n.initialBalance.Set(currentBalance)
	return nil
}

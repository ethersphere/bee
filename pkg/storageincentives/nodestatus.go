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
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
	"math/big"
	"sync"
)

const redistributionStatusKey = "redistribution_state_"
const loggerNameNode = "nodestatus"

type NodeState struct {
	stateStore     storage.StateStorer
	erc20Service   erc20.Service
	logger         log.Logger
	overlay        swarm.Address
	mtx            sync.Mutex
	nodeStatus     NodeStatus
	currentBalance *big.Int
	contract       redistribution.Contract
}

// NodeStatus provide internal status of the nodes in the redistribution game
type NodeStatus struct {
	Phase  PhaseType `json:"phase"`
	State  State     `json:"state"`
	Round  uint64    `json:"round"`
	Block  uint64    `json:"block"`
	Reward *big.Int  `json:"reward"`
	Fees   *big.Int  `json:"fees"`
}

type State int

const (
	winner State = iota + 1
	frozen
	idle
)

func (s State) String() string {
	switch s {
	case winner:
		return "winner"
	case frozen:
		return "frozen"
	case idle:
		return "idle"
	default:
		return "unknown"
	}
}

func NewNode(logger log.Logger, stateStore storage.StateStorer, erc20Service erc20.Service, contract redistribution.Contract) NodeState {
	return NodeState{
		stateStore:     stateStore,
		erc20Service:   erc20Service,
		logger:         logger.WithName(loggerNameNode).Register(),
		currentBalance: big.NewInt(0),
		contract:       contract,
		nodeStatus: NodeStatus{
			Round:  0,
			Block:  0,
			Reward: nil,
			Fees:   nil,
		},
	}
}

func (n *NodeState) SetState(s State) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.nodeStatus.State = s
}

func (n *NodeState) SetPhase(p PhaseType) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.nodeStatus.Phase = p
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

// AddFee sets the internal node status
func (n *NodeState) AddFee() {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	fee := n.contract.Fee()
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
		n.nodeStatus.Reward = currentBalance.Sub(currentBalance, n.currentBalance)
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
	n.currentBalance.Set(currentBalance)
	return nil
}

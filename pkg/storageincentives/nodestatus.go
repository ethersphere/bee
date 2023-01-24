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

const redistributionStatusKey = "redistribution_state"
const loggerNameNode = "nodestatus"

type NodeState struct {
	stateStore     storage.StateStorer
	erc20Service   erc20.Service
	logger         log.Logger
	overlay        swarm.Address
	mtx            sync.Mutex
	status         Status
	currentBalance *big.Int
	contract       redistribution.Contract
}

// Status provide internal status of the nodes in the redistribution game
type Status struct {
	Phase        PhaseType `json:"phase"`
	State        State     `json:"state"`
	Round        uint64    `json:"round"`
	LastWonRound uint64    `json:"lastWonRound"`
	Block        uint64    `json:"block"`
	Reward       *big.Int  `json:"reward"`
	Fees         *big.Int  `json:"fees"`
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
		status: Status{
			Round:  0,
			Block:  0,
			Reward: nil,
			Fees:   nil,
		},
	}
}

func (n *NodeState) SetCurrentEvent(p PhaseType, r uint64, b uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.Phase = p
	n.status.Round = r
	n.status.Block = b
	n.status.State = idle
	n.SaveStatus()
}

func (n *NodeState) SetState(s State) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.State = s
}

func (n *NodeState) SetLastWonRound(r uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.LastWonRound = r
}

func (n *NodeState) SetBlock(b uint64) {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	n.status.Block = b
}

// AddFee sets the internal node status
func (n *NodeState) AddFee() {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	fee := n.contract.Fee()
	if fee != nil {
		n.status.Fees.Add(n.status.Fees, fee)
		n.SaveStatus()
	}
}

// CalculateWinnerReward calculates the reward for the winner
func (n *NodeState) CalculateWinnerReward(ctx context.Context) error {
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

// SaveStatus saves the node status to the database
func (n *NodeState) SaveStatus() {
	n.mtx.Lock()
	defer n.mtx.Unlock()
	err := n.stateStore.Put(redistributionStatusKey, n.status)
	if err != nil {
		n.logger.Error(err, "error saving node status")
		return
	}
}

// Status returns the node status
func (n *NodeState) Status() (*Status, error) {
	status := new(Status)
	if err := n.stateStore.Get(redistributionStatusKey, status); err != nil {
		return nil, err
	}
	return status, nil
}

func (n *NodeState) SetBalance(ctx context.Context) error {
	// get current balance
	currentBalance, err := n.erc20Service.BalanceOf(ctx, common.HexToAddress(n.overlay.String()))
	if err != nil {
		n.logger.Debug("error getting balance", "error", err, "overly address", n.overlay.String())
		return err
	}
	n.currentBalance.Set(currentBalance)
	return nil
}

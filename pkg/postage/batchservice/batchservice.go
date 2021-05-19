// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
)

type batchService struct {
	storer   postage.Storer
	logger   logging.Logger
	listener postage.Listener
}

type Interface interface {
	postage.EventUpdater
}

// New will create a new BatchService.
func New(storer postage.Storer, logger logging.Logger, listener postage.Listener) Interface {
	return &batchService{storer, logger, listener}
}

// Create will create a new batch with the given ID, owner value and depth and
// stores it in the BatchStore.
func (svc *batchService) Create(id, owner []byte, normalisedBalance *big.Int, depth uint8) error {
	b := &postage.Batch{
		ID:    id,
		Owner: owner,
		Value: big.NewInt(0),
		Start: svc.storer.GetChainState().Block,
		Depth: depth,
	}

	err := svc.storer.Put(b, normalisedBalance, depth)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("batch service: created batch id %s", hex.EncodeToString(b.ID))
	return nil
}

// TopUp implements the EventUpdater interface. It tops ups a batch with the
// given ID with the given amount.
func (svc *batchService) TopUp(id []byte, normalisedBalance *big.Int) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	err = svc.storer.Put(b, normalisedBalance, b.Depth)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("batch service: topped up batch id %s from %v to %v", hex.EncodeToString(b.ID), b.Value, normalisedBalance)
	return nil
}

// UpdateDepth implements the EventUpdater inteface. It sets the new depth of a
// batch with the given ID.
func (svc *batchService) UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	err = svc.storer.Put(b, normalisedBalance, depth)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("batch service: updated depth of batch id %s from %d to %d", hex.EncodeToString(b.ID), b.Depth, depth)
	return nil
}

// UpdatePrice implements the EventUpdater interface. It sets the current
// price from the chain in the service chain state.
func (svc *batchService) UpdatePrice(price *big.Int) error {
	cs := svc.storer.GetChainState()
	cs.CurrentPrice = price
	if err := svc.storer.PutChainState(cs); err != nil {
		return fmt.Errorf("put chain state: %w", err)
	}

	svc.logger.Debugf("batch service: updated chain price to %s", price)
	return nil
}

func (svc *batchService) UpdateBlockNumber(blockNumber uint64) error {
	cs := svc.storer.GetChainState()
	if blockNumber == cs.Block {
		return nil
	}
	diff := big.NewInt(0).SetUint64(blockNumber - cs.Block)

	cs.TotalAmount.Add(cs.TotalAmount, diff.Mul(diff, cs.CurrentPrice))
	cs.Block = blockNumber
	if err := svc.storer.PutChainState(cs); err != nil {
		return fmt.Errorf("put chain state: %w", err)
	}

	svc.logger.Debugf("batch service: updated block height to %d", blockNumber)
	return nil
}

func (svc *batchService) Start(startBlock uint64) <-chan struct{} {
	cs := svc.storer.GetChainState()
	if cs.Block > startBlock {
		startBlock = cs.Block
	}
	return svc.listener.Listen(startBlock+1, svc)
}

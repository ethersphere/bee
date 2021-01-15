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
	cs       *postage.ChainState
	storer   postage.Storer
	logger   logging.Logger
	listener postage.Listener
}

type Interface interface {
	postage.EventUpdater
	Start() error
}

// New will create a new BatchService.
func New(storer postage.Storer, logger logging.Logger, listener postage.Listener) (Interface, error) {
	b := &batchService{
		storer:   storer,
		logger:   logger,
		listener: listener,
	}

	cs, err := storer.GetChainState()
	if err != nil {
		return nil, fmt.Errorf("get chain state: %w", err)
	}
	b.cs = cs
	return b, nil
}

func (svc *batchService) Start() error {
	cs, err := svc.storer.GetChainState()
	if err != nil {
		return fmt.Errorf("get chain state: %w", err)
	}
	svc.listener.Listen(cs.Block+1, svc)
	return nil
}

// Create will create a new batch with the given ID, owner value and depth and
// stores it in the BatchStore.
func (svc *batchService) Create(id, owner []byte, normalisedBalance *big.Int, depth uint8) error {
	b := &postage.Batch{
		ID:    id,
		Owner: owner,
		Value: normalisedBalance,
		Start: svc.cs.Block,
		Depth: depth,
	}

	err := svc.storer.Put(b)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("created batch id %s", hex.EncodeToString(b.ID))
	return nil
}

// TopUp implements the EventUpdater interface. It tops ups a batch with the
// given ID with the given amount.
func (svc *batchService) TopUp(id []byte, normalisedBalance *big.Int) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	b.Value.Set(normalisedBalance)

	err = svc.storer.Put(b)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("topped up batch id %s with %v", hex.EncodeToString(b.ID), b.Value)
	return nil
}

// UpdateDepth implements the EventUpdater inteface. It sets the new depth of a
// batch with the given ID.
func (svc *batchService) UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	b.Depth = depth
	b.Value.Set(normalisedBalance)

	err = svc.storer.Put(b)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("updated depth of batch id %s to %d", hex.EncodeToString(b.ID), b.Depth)
	return nil
}

// UpdatePrice implements the EventUpdater interface. It sets the current
// price from the chain in the service chain state.
func (svc *batchService) UpdatePrice(price *big.Int) error {
	svc.cs.Price = price

	if err := svc.storer.PutChainState(svc.cs); err != nil {
		return fmt.Errorf("put chain state: %w", err)
	}

	svc.logger.Debugf("updated chain price to %s", svc.cs.Price)
	return nil
}

func (svc *batchService) UpdateBlockNumber(blockNumber uint64) error {
	svc.cs.Block = blockNumber

	if err := svc.storer.PutChainState(svc.cs); err != nil {
		return fmt.Errorf("put chain state: %w", err)
	}

	svc.logger.Debugf("updated block height to %d", svc.cs.Block)
	return nil
}

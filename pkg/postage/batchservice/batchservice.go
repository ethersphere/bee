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

// BatchService implements the EventUpdater interface.
type BatchService struct {
	cs     *postage.ChainState
	storer postage.Storer
	logger logging.Logger
}

// New will create a new BatchService.
func New(storer postage.Storer, logger logging.Logger) (postage.EventUpdater, error) {
	b := &BatchService{
		storer: storer,
		logger: logger,
	}

	cs, err := storer.GetChainState()
	if err != nil {
		return nil, fmt.Errorf("get chain state: %w", err)
	}
	b.cs = cs

	return b, nil
}

// Create will create a new batch and store it in the BatchStore.
func (svc *BatchService) Create(id, owner []byte, value *big.Int, depth uint8) error {
	b := &postage.Batch{
		ID:    id,
		Owner: owner,
		Value: value,
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
// given ID with the given amount of BZZ.
func (svc *BatchService) TopUp(id []byte, amount *big.Int) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	b.Value.Add(b.Value, amount)

	err = svc.storer.Put(b)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("topped up batch id %s with %v", hex.EncodeToString(b.ID), b.Value)
	return nil
}

// UpdateDepth implements the EventUpdater inteface. It sets the new depth of a
// batch with the given ID.
func (svc *BatchService) UpdateDepth(id []byte, depth uint8) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	b.Depth = depth

	err = svc.storer.Put(b)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	svc.logger.Debugf("updated depth of batch id %s to %d", hex.EncodeToString(b.ID), b.Depth)
	return nil
}

// UpdatePrice implements the EventUpdater interface. It sets the current
// price from the chain in the service chain state.
func (svc *BatchService) UpdatePrice(price *big.Int) error {
	svc.cs.Price = price

	if err := svc.storer.PutChainState(svc.cs); err != nil {
		return fmt.Errorf("put chain state: %w", err)
	}

	svc.logger.Debugf("updated chain price to %s", svc.cs.Price)
	return nil
}

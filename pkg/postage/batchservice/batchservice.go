// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"golang.org/x/crypto/sha3"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "batchservice"

const (
	dirtyDBKey    = "batchservice_dirty_db"
	checksumDBKey = "batchservice_checksum"
)

var ErrZeroValueBatch = errors.New("low balance batch")

type batchService struct {
	stateStore    storage.StateStorer
	storer        postage.Storer
	logger        log.Logger
	listener      postage.Listener
	owner         []byte
	batchListener postage.BatchEventListener

	checksum hash.Hash // checksum hasher
	resync   bool
}

type Interface interface {
	postage.EventUpdater
}

// New will create a new BatchService.
func New(
	stateStore storage.StateStorer,
	storer postage.Storer,
	logger log.Logger,
	listener postage.Listener,
	owner []byte,
	batchListener postage.BatchEventListener,
	checksumFunc func() hash.Hash,
	resync bool,
) (Interface, error) {
	if checksumFunc == nil {
		checksumFunc = sha3.New256
	}
	var (
		b   string
		sum = checksumFunc()
	)

	dirty := false
	err := stateStore.Get(dirtyDBKey, &dirty)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	if resync {
		if err := stateStore.Delete(checksumDBKey); err != nil {
			return nil, err
		}
	} else if !dirty {
		if err := stateStore.Get(checksumDBKey, &b); err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				return nil, err
			}
		} else {
			s, err := hex.DecodeString(b)
			if err != nil {
				return nil, err
			}
			n, err := sum.Write(s)
			if err != nil {
				return nil, err
			}
			if n != len(s) {
				return nil, errors.New("batchstore checksum init")
			}
		}
	}

	return &batchService{stateStore, storer, logger.WithName(loggerName).Register(), listener, owner, batchListener, sum, resync}, nil
}

// Create will create a new batch with the given ID, owner value and depth and
// stores it in the BatchedStore.
func (svc *batchService) Create(id, owner []byte, totalAmout, normalisedBalance *big.Int, depth, bucketDepth uint8, immutable bool, txHash common.Hash) error {
	// don't add batches which have value which equals total cumulative
	// payout or that are going to expire already within the next couple of blocks
	val := big.NewInt(0).Add(svc.storer.GetChainState().TotalAmount, svc.storer.GetChainState().CurrentPrice)
	if normalisedBalance.Cmp(val) <= 0 {
		// don't do anything
		return fmt.Errorf("batch service: batch %x: %w", id, ErrZeroValueBatch)
	}
	batch := &postage.Batch{
		ID:          id,
		Owner:       owner,
		Value:       normalisedBalance,
		Start:       svc.storer.GetChainState().Block,
		Depth:       depth,
		BucketDepth: bucketDepth,
		Immutable:   immutable,
	}

	err := svc.storer.Save(batch)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	amount := big.NewInt(0).Div(totalAmout, big.NewInt(int64(1<<(batch.Depth))))

	if bytes.Equal(svc.owner, owner) && svc.batchListener != nil {
		if err := svc.batchListener.HandleCreate(batch, amount); err != nil {
			return fmt.Errorf("create batch: %w", err)
		}
	}

	cs, err := svc.updateChecksum(txHash)
	if err != nil {
		return fmt.Errorf("update checksum: %w", err)
	}

	svc.logger.Debug("batch created", "batch_id", hex.EncodeToString(batch.ID), "tx", txHash, "tx_checksum", cs)
	return nil
}

// TopUp implements the EventUpdater interface. It tops ups a batch with the
// given ID with the given amount.
func (svc *batchService) TopUp(id []byte, totalAmout, normalisedBalance *big.Int, txHash common.Hash) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	err = svc.storer.Update(b, normalisedBalance, b.Depth)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	topUpAmount := big.NewInt(0).Div(totalAmout, big.NewInt(int64(1<<(b.Depth))))

	if bytes.Equal(svc.owner, b.Owner) && svc.batchListener != nil {
		svc.batchListener.HandleTopUp(id, topUpAmount)
	}

	cs, err := svc.updateChecksum(txHash)
	if err != nil {
		return fmt.Errorf("update checksum: %w", err)
	}

	svc.logger.Debug("topped up batch", "batch_id", hex.EncodeToString(b.ID), "old_value", b.Value, "new_value", normalisedBalance, "tx", txHash, "tx_checksum", cs)
	return nil
}

// UpdateDepth implements the EventUpdater interface. It sets the new depth of a
// batch with the given ID.
func (svc *batchService) UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int, txHash common.Hash) error {
	b, err := svc.storer.Get(id)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	err = svc.storer.Update(b, normalisedBalance, depth)
	if err != nil {
		return fmt.Errorf("put: %w", err)
	}

	if bytes.Equal(svc.owner, b.Owner) && svc.batchListener != nil {
		svc.batchListener.HandleDepthIncrease(id, depth)
	}

	cs, err := svc.updateChecksum(txHash)
	if err != nil {
		return fmt.Errorf("update checksum: %w", err)
	}

	svc.logger.Debug("updated depth of batch", "batch_id", hex.EncodeToString(b.ID), "old_depth", b.Depth, "new_depth", depth, "tx", txHash, "tx_checksum", cs)
	return nil
}

// UpdatePrice implements the EventUpdater interface. It sets the current
// price from the chain in the service chain state.
func (svc *batchService) UpdatePrice(price *big.Int, txHash common.Hash) error {
	cs := svc.storer.GetChainState()
	cs.CurrentPrice = price
	if err := svc.storer.PutChainState(cs); err != nil {
		return fmt.Errorf("put chain state: %w", err)
	}

	sum, err := svc.updateChecksum(txHash)
	if err != nil {
		return fmt.Errorf("update checksum: %w", err)
	}

	svc.logger.Debug("updated chain price", "new_price", price, "tx_hash", txHash, "tx_checksum", sum)
	return nil
}

func (svc *batchService) UpdateBlockNumber(blockNumber uint64) error {
	cs := svc.storer.GetChainState()
	if blockNumber == cs.Block {
		return nil
	}
	if blockNumber < cs.Block {
		return fmt.Errorf("batch service: block number moved backwards from %d to %d", cs.Block, blockNumber)
	}
	diff := big.NewInt(0).SetUint64(blockNumber - cs.Block)

	cs.TotalAmount.Add(cs.TotalAmount, diff.Mul(diff, cs.CurrentPrice))
	cs.Block = blockNumber
	if err := svc.storer.PutChainState(cs); err != nil {
		return fmt.Errorf("put chain state: %w", err)
	}

	svc.logger.Debug("block height updated", "new_block", blockNumber)
	return nil
}
func (svc *batchService) TransactionStart() error {
	return svc.stateStore.Put(dirtyDBKey, true)
}
func (svc *batchService) TransactionEnd() error {
	return svc.stateStore.Delete(dirtyDBKey)
}

var ErrInterruped = errors.New("postage sync interrupted")

func (svc *batchService) Start(ctx context.Context, startBlock uint64, initState *postage.ChainSnapshot) (err error) {
	dirty := false
	err = svc.stateStore.Get(dirtyDBKey, &dirty)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	if dirty || svc.resync || initState != nil {

		if dirty {
			svc.logger.Warning("batch service: dirty shutdown detected, resetting batch store")
		} else {
			svc.logger.Warning("batch service: resync requested, resetting batch store")
		}

		if err := svc.storer.Reset(); err != nil {
			return err
		}
		if err := svc.stateStore.Delete(dirtyDBKey); err != nil {
			return err
		}
		svc.logger.Warning("batch service: batch store has been reset. your node will now resync chain data. this might take a while...")
	}

	cs := svc.storer.GetChainState()
	if cs.Block > startBlock {
		startBlock = cs.Block
	}

	if initState != nil && initState.LastBlockNumber > startBlock {
		startBlock = initState.LastBlockNumber
	}

	syncedChan := svc.listener.Listen(ctx, startBlock+1, svc, initState)

	return <-syncedChan
}

// updateChecksum updates the batchservice checksum once an event gets
// processed. It swaps the existing checksum which is in the hasher
// with the new checksum and persists it in the statestore.
func (svc *batchService) updateChecksum(txHash common.Hash) (string, error) {
	n, err := svc.checksum.Write(txHash.Bytes())
	if err != nil {
		return "", err
	}
	if l := len(txHash.Bytes()); l != n {
		return "", fmt.Errorf("update checksum wrote %d bytes but want %d bytes", n, l)
	}
	s := svc.checksum.Sum(nil)
	svc.checksum.Reset()
	n, err = svc.checksum.Write(s)
	if err != nil {
		return "", err
	}
	if l := len(s); l != n {
		return "", fmt.Errorf("swap checksum wrote %d bytes but want %d bytes", n, l)
	}

	b := hex.EncodeToString(s)

	return b, svc.stateStore.Put(checksumDBKey, b)
}

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

	// snapshotResumeBlock is the chain block height the store was rebuilt to
	// from a postage snapshot. When set, live sync resumes from here.
	snapshotResumeBlock uint64
}

type Interface interface {
	postage.EventUpdater
}

// Snapshot carries the optional inputs needed to rebuild the batch store from a
// postage snapshot. When passed to New (non-nil), the store is reset and
// replayed from the snapshot before the service is returned, and live sync
// later resumes from the snapshot's block height. New takes ownership of
// Listener and closes it once the snapshot has been replayed.
type Snapshot struct {
	// Listener replays the snapshot's events into the batch store.
	Listener postage.Listener
	// StartBlock is the block height from which the snapshot is replayed (the
	// postage contract start block).
	StartBlock uint64
	// ResumeBlock is the block height the snapshot reached, from which live sync
	// resumes once the replay completes.
	ResumeBlock uint64
}

// New will create a new BatchService.
//
// The batch store is reset here, in the constructor, when a resync was requested
// or a dirty shutdown was detected. A provided snapshot is then replayed onto the
// (possibly reset) store; if that replay fails the store is reset again so live
// sync can rebuild it from the chain. Start never resets the store. The returned
// bool reports whether the snapshot was replayed successfully.
func New(
	ctx context.Context,
	stateStore storage.StateStorer,
	storer postage.Storer,
	logger log.Logger,
	listener postage.Listener,
	owner []byte,
	batchListener postage.BatchEventListener,
	checksumFunc func() hash.Hash,
	snapshot *Snapshot,
	resync bool,
) (Interface, bool, error) {
	if checksumFunc == nil {
		checksumFunc = sha3.New256
	}
	var (
		b   string
		sum = checksumFunc()
	)

	logger = logger.WithName(loggerName).Register()

	dirty := false
	if err := stateStore.Get(dirtyDBKey, &dirty); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, false, err
	}
	if dirty {
		logger.Warning("batch service: dirty shutdown detected, resetting batch store")
	}

	if resync {
		logger.Warning("batch service: resync requested, resetting batch store")
		if err := stateStore.Delete(checksumDBKey); err != nil {
			return nil, false, err
		}
	} else if !dirty {
		if err := stateStore.Get(checksumDBKey, &b); err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				return nil, false, err
			}
		} else {
			s, err := hex.DecodeString(b)
			if err != nil {
				return nil, false, err
			}
			n, err := sum.Write(s)
			if err != nil {
				return nil, false, err
			}
			if n != len(s) {
				return nil, false, errors.New("batchstore checksum init")
			}
		}
	}

	bs := &batchService{
		stateStore:    stateStore,
		storer:        storer,
		logger:        logger,
		listener:      listener,
		owner:         owner,
		batchListener: batchListener,
		checksum:      sum,
	}

	// Reset the store once, here, when a resync was requested or a dirty shutdown
	// was detected (both already logged above). A snapshot is then replayed onto
	// the store; Start does not reset, so this is the only unconditional reset.
	if dirty || resync {
		if err := bs.reset(); err != nil {
			return nil, false, err
		}
	}

	snapshotLoaded := false
	if snapshot != nil {
		if err := bs.loadSnapshot(ctx, snapshot); err != nil {
			logger.Error(err, "failed to start batch service from snapshot, continuing outside snapshot block...")
			// A partial replay may have written to (and dirtied) the store, so
			// reset it again to rebuild cleanly from the chain during live sync.
			if err := bs.reset(); err != nil {
				return nil, false, err
			}
		} else {
			snapshotLoaded = true
		}
	}

	return bs, snapshotLoaded, nil
}

// reset wipes the batch store so it can be rebuilt from scratch. It runs only
// during New. The reason for the reset is logged by the caller at the point of
// detection, so the intent is recorded even if the reset itself then fails.
func (svc *batchService) reset() error {
	if err := svc.storer.Reset(); err != nil {
		return err
	}
	if err := svc.stateStore.Delete(dirtyDBKey); err != nil {
		return err
	}
	svc.logger.Warning("batch service: batch store has been reset. your node will now resync chain data. this might take a while...")

	return nil
}

// loadSnapshot rebuilds the (already reset) store from a postage snapshot by
// replaying its events and records the block height to resume live sync from.
func (svc *batchService) loadSnapshot(ctx context.Context, snapshot *Snapshot) error {
	defer func() {
		if err := snapshot.Listener.Close(); err != nil {
			svc.logger.Error(err, "failed to close event listener (snapshot) failure")
		}
	}()

	startBlock := snapshot.StartBlock
	if cs := svc.storer.GetChainState(); cs.Block > startBlock {
		startBlock = cs.Block
	}

	if err := <-snapshot.Listener.Listen(ctx, startBlock+1, svc); err != nil {
		return err
	}

	svc.snapshotResumeBlock = snapshot.ResumeBlock

	return nil
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

func (svc *batchService) Start(ctx context.Context, startBlock uint64) (err error) {
	// The store reset already happened in New, so Start only drives live sync.
	cs := svc.storer.GetChainState()
	if cs.Block > startBlock {
		startBlock = cs.Block
	}
	// When the store was rebuilt from a snapshot, resume live sync from the
	// snapshot's block height rather than the requested start block.
	if svc.snapshotResumeBlock > startBlock {
		startBlock = svc.snapshotResumeBlock
	}

	syncedChan := svc.listener.Listen(ctx, startBlock+1, svc)

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

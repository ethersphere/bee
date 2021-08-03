// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"io"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
)

var ErrTransactionCancelled = errors.New("transaction cancelled")
var ErrMonitorClosed = errors.New("monitor closed")

// Monitor is a nonce-based watcher for transaction confirmations.
// Instead of watching transactions individually, the senders nonce is monitored and transactions are checked based on this.
// The idea is that if the nonce is still lower than that of a pending transaction, there is no point in actually checking the transaction for a receipt.
// At the same time if the nonce was already used and this was a few blocks ago we can reasonably assume that it will never confirm.
type Monitor interface {
	io.Closer
	// WatchTransaction watches the transaction until either there is 1 confirmation or a competing transaction with cancellationDepth confirmations.
	WatchTransaction(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error)
}
type transactionMonitor struct {
	lock       sync.Mutex
	ctx        context.Context    // context which is used for all backend calls
	cancelFunc context.CancelFunc // function to cancel the above context
	wg         sync.WaitGroup

	logger  logging.Logger
	backend Backend
	sender  common.Address // sender of transactions which this instance can monitor

	pollingInterval   time.Duration // time between checking for new blocks
	cancellationDepth uint64        // number of blocks until considering a tx cancellation final

	watches    map[*transactionWatch]struct{} // active watches
	watchAdded chan struct{}                  // channel to trigger instant pending check
}

type transactionWatch struct {
	receiptC chan types.Receipt // channel to which the receipt will be written once available
	errC     chan error         // error channel (primarily for cancelled transactions)

	txHash common.Hash // hash of the transaction to watch
	nonce  uint64      // nonce of the transaction to watch
}

func NewMonitor(logger logging.Logger, backend Backend, sender common.Address, pollingInterval time.Duration, cancellationDepth uint64) Monitor {
	ctx, cancelFunc := context.WithCancel(context.Background())

	t := &transactionMonitor{
		ctx:        ctx,
		cancelFunc: cancelFunc,
		logger:     logger,
		backend:    backend,
		sender:     sender,

		pollingInterval:   pollingInterval,
		cancellationDepth: cancellationDepth,

		watches:    make(map[*transactionWatch]struct{}),
		watchAdded: make(chan struct{}, 1),
	}

	t.wg.Add(1)
	go t.watchPending()

	return t
}

func (tm *transactionMonitor) WatchTransaction(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
	tm.lock.Lock()
	defer tm.lock.Unlock()

	// these channels will be written to at most once
	// buffer size is 1 to avoid blocking in the watch loop
	receiptC := make(chan types.Receipt, 1)
	errC := make(chan error, 1)

	tm.watches[&transactionWatch{
		receiptC: receiptC,
		errC:     errC,
		txHash:   txHash,
		nonce:    nonce,
	}] = struct{}{}

	select {
	case tm.watchAdded <- struct{}{}:
	default:
	}

	tm.logger.Tracef("starting to watch transaction %x with nonce %d", txHash, nonce)

	return receiptC, errC, nil
}

// main watch loop
func (tm *transactionMonitor) watchPending() {
	defer tm.wg.Done()
	defer func() {
		tm.lock.Lock()
		defer tm.lock.Unlock()

		for watch := range tm.watches {
			watch.errC <- ErrMonitorClosed
		}
	}()

	var (
		lastBlock uint64 = 0
		added     bool   // flag if this iteration was triggered by the watchAdded channel
	)

	for {
		added = false
		select {
		// if a new watch has been added check again without waiting
		case <-tm.watchAdded:
			added = true
		// otherwise wait
		case <-time.After(tm.pollingInterval):
		// if the main context is cancelled terminate
		case <-tm.ctx.Done():
			return
		}

		// if there are no watched transactions there is nothing to do
		if !tm.hasWatches() {
			continue
		}

		// switch to new head subscriptions once websockets are the norm
		block, err := tm.backend.BlockNumber(tm.ctx)
		if err != nil {
			tm.logger.Errorf("could not get block number: %v", err)
			continue
		} else if block <= lastBlock && !added {
			// if the block number is not higher than before there is nothing todo
			// unless a watch was added in which case we will do the check anyway
			// in the rare case where a block was reorged and the new one is the first to contain our tx we wait an extra block
			continue
		}

		if err := tm.checkPending(block); err != nil {
			tm.logger.Tracef("error while checking pending transactions: %v", err)
			continue
		}
		lastBlock = block
	}
}

type watchesWithNonce struct {
	watches []*transactionWatch
	nonce   uint64
}

// potentiallyConfirmedTxs returns all watches with nonce less than what was specified
func (tm *transactionMonitor) potentiallyConfirmedTxs(nonce uint64) (watches map[common.Hash]*watchesWithNonce) {
	tm.lock.Lock()
	defer tm.lock.Unlock()

	checkTxs := make(map[common.Hash]*watchesWithNonce)
	for watch := range tm.watches {
		if watch.nonce < nonce {
			if c, ok := checkTxs[watch.txHash]; ok {
				c.watches = append(c.watches, watch)
			} else {
				checkTxs[watch.txHash] = &watchesWithNonce{
					nonce:   watch.nonce,
					watches: []*transactionWatch{watch},
				}
			}
		}
	}

	return checkTxs
}

func (tm *transactionMonitor) hasWatches() bool {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	return len(tm.watches) > 0
}

// check pending checks the given block (number) for confirmed or cancelled transactions
func (tm *transactionMonitor) checkPending(block uint64) error {
	nonce, err := tm.backend.NonceAt(tm.ctx, tm.sender, new(big.Int).SetUint64(block))
	if err != nil {
		return err
	}

	// transactions with a nonce lower or equal to what is found on-chain are either confirmed or (at least temporarily) cancelled
	checkTxs := tm.potentiallyConfirmedTxs(nonce)

	var confirmedTxs []*types.Receipt
	var potentiallyCancelledTxs []common.Hash
	for txHash := range checkTxs {
		receipt, err := tm.backend.TransactionReceipt(tm.ctx, txHash)
		if receipt != nil {
			// if we have a receipt we have a confirmation
			confirmedTxs = append(confirmedTxs, receipt)
		} else if err == nil || errors.Is(err, ethereum.NotFound) {
			// if both err and receipt are nil, there is no receipt
			// we also match for the special error "not found" that some clients return
			// the reason why we consider this only potentially cancelled is to catch cases where after a reorg the original transaction wins
			potentiallyCancelledTxs = append(potentiallyCancelledTxs, txHash)
		} else {
			// any other error is probably a real error
			return err
		}
	}

	// mark all transactions without receipt whose nonce was already used at least cancellationDepth blocks ago as cancelled
	var cancelledTxs []common.Hash
	if len(potentiallyCancelledTxs) > 0 {
		oldNonce, err := tm.backend.NonceAt(tm.ctx, tm.sender, new(big.Int).SetUint64(block-tm.cancellationDepth))
		if err != nil {
			return err
		}

		for _, txHash := range potentiallyCancelledTxs {
			// oldNonce is the nonce of the next tx that could have been included
			// if this was already larger in the past our transaction becomes impossible
			if checkTxs[txHash].nonce <= oldNonce {
				cancelledTxs = append(cancelledTxs, txHash)
			}
		}
	}

	// notify the subscribers and remove watches for confirmed or cancelled transactions
	tm.lock.Lock()
	defer tm.lock.Unlock()

	for _, receipt := range confirmedTxs {
		for _, watch := range checkTxs[receipt.TxHash].watches {
			watch.receiptC <- *receipt
			delete(tm.watches, watch)
		}
	}

	for _, txHash := range cancelledTxs {
		for _, watch := range checkTxs[txHash].watches {
			watch.errC <- ErrTransactionCancelled
			delete(tm.watches, watch)
		}
	}
	return nil
}

func (tm *transactionMonitor) Close() error {
	tm.cancelFunc()
	tm.wg.Wait()
	return nil
}

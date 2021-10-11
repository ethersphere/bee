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

	watchesByNonce map[uint64]map[common.Hash][]transactionWatch // active watches grouped by nonce and tx hash
	watchAdded     chan struct{}                                 // channel to trigger instant pending check
}

type transactionWatch struct {
	receiptC chan types.Receipt // channel to which the receipt will be written once available
	errC     chan error         // error channel (primarily for cancelled transactions)
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

		watchesByNonce: make(map[uint64]map[common.Hash][]transactionWatch),
		watchAdded:     make(chan struct{}, 1),
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

	if _, ok := tm.watchesByNonce[nonce]; !ok {
		tm.watchesByNonce[nonce] = make(map[common.Hash][]transactionWatch)
	}

	tm.watchesByNonce[nonce][txHash] = append(tm.watchesByNonce[nonce][txHash], transactionWatch{
		receiptC: receiptC,
		errC:     errC,
	})

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

		for _, watches := range tm.watchesByNonce {
			for _, txMap := range watches {
				for _, watch := range txMap {
					select {
					case watch.errC <- ErrMonitorClosed:
					default:
					}
				}
			}
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

// potentiallyConfirmedTxWatches returns all watches with nonce less than what was specified
func (tm *transactionMonitor) potentiallyConfirmedTxWatches(nonce uint64) (watches map[uint64]map[common.Hash][]transactionWatch) {
	tm.lock.Lock()
	defer tm.lock.Unlock()

	potentiallyConfirmedTxWatches := make(map[uint64]map[common.Hash][]transactionWatch)
	for n, watches := range tm.watchesByNonce {
		if n < nonce {
			potentiallyConfirmedTxWatches[n] = watches
		}
	}

	return potentiallyConfirmedTxWatches
}

func (tm *transactionMonitor) hasWatches() bool {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	return len(tm.watchesByNonce) > 0
}

// check pending checks the given block (number) for confirmed or cancelled transactions
func (tm *transactionMonitor) checkPending(block uint64) error {
	nonce, err := tm.backend.NonceAt(tm.ctx, tm.sender, new(big.Int).SetUint64(block))
	if err != nil {
		return err
	}

	// transactions with a nonce lower or equal to what is found on-chain are either confirmed or (at least temporarily) cancelled
	potentiallyConfirmedTxWatches := tm.potentiallyConfirmedTxWatches(nonce)

	confirmedNonces := make(map[uint64]*types.Receipt)
	var cancelledNonces []uint64
	for nonceGroup, watchMap := range potentiallyConfirmedTxWatches {
		for txHash := range watchMap {
			receipt, err := tm.backend.TransactionReceipt(tm.ctx, txHash)
			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					// if both err and receipt are nil, there is no receipt
					// the reason why we consider this only potentially cancelled is to catch cases where after a reorg the original transaction wins
					continue
				}
				return err
			}
			if receipt != nil {
				// if we have a receipt we have a confirmation
				confirmedNonces[nonceGroup] = receipt
			}
		}
	}

	for nonceGroup := range potentiallyConfirmedTxWatches {
		if _, ok := confirmedNonces[nonceGroup]; ok {
			continue
		}

		oldNonce, err := tm.backend.NonceAt(tm.ctx, tm.sender, new(big.Int).SetUint64(block-tm.cancellationDepth))
		if err != nil {
			return err
		}

		if nonceGroup < oldNonce {
			cancelledNonces = append(cancelledNonces, nonceGroup)
		}
	}

	// notify the subscribers and remove watches for confirmed or cancelled transactions
	tm.lock.Lock()
	defer tm.lock.Unlock()

	for nonce, receipt := range confirmedNonces {
		for txHash, watches := range potentiallyConfirmedTxWatches[nonce] {
			if receipt.TxHash == txHash {
				for _, watch := range watches {
					select {
					case watch.receiptC <- *receipt:
					default:
					}
				}
			} else {
				for _, watch := range watches {
					select {
					case watch.errC <- ErrTransactionCancelled:
					default:
					}
				}
			}
		}
		delete(tm.watchesByNonce, nonce)
	}

	for _, nonce := range cancelledNonces {
		for _, watches := range tm.watchesByNonce[nonce] {
			for _, watch := range watches {
				select {
				case watch.errC <- ErrTransactionCancelled:
				default:
				}
			}
		}
		delete(tm.watchesByNonce, nonce)
	}

	return nil
}

func (tm *transactionMonitor) Close() error {
	tm.cancelFunc()
	tm.wg.Wait()
	return nil
}

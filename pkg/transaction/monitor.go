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
	"github.com/ethersphere/bee/v2/pkg/log"
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

	logger  log.Logger
	backend Backend
	sender  common.Address // sender of transactions which this instance can monitor

	pollingInterval   time.Duration // time between checking for new blocks
	cancellationDepth uint64        // number of blocks until considering a tx cancellation final

	watchesByNonce map[uint64]map[common.Hash][]transactionWatch // active watches grouped by nonce and tx hash
	watchAdded     chan struct{}                                 // channel to trigger instant pending check
}

type transactionWatch struct {
	start    time.Time
	receiptC chan types.Receipt // channel to which the receipt will be written once available
	errC     chan error         // error channel (primarily for cancelled transactions)
}

func NewMonitor(logger log.Logger, backend Backend, sender common.Address, pollingInterval time.Duration, cancellationDepth uint64) Monitor {
	ctx, cancelFunc := context.WithCancel(context.Background())

	t := &transactionMonitor{
		ctx:        ctx,
		cancelFunc: cancelFunc,
		logger:     logger.WithName(loggerName).Register(),
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
	loggerV1 := tm.logger.V(1).Register()

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
		start:    time.Now(),
		receiptC: receiptC,
		errC:     errC,
	})

	select {
	case tm.watchAdded <- struct{}{}:
	default:
	}

	loggerV1.Debug("starting to watch transaction", "tx", txHash, "nonce", nonce)

	return receiptC, errC, nil
}

// main watch loop
func (tm *transactionMonitor) watchPending() {
	loggerV1 := tm.logger.V(1).Register()

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
			tm.logger.Error(err, "could not get block number")
			continue
		} else if block <= lastBlock && !added {
			// if the block number is not higher than before there is nothing todo
			// unless a watch was added in which case we will do the check anyway
			// in the rare case where a block was reorged and the new one is the first to contain our tx we wait an extra block
			continue
		}

		if err := tm.checkPending(block); err != nil {
			loggerV1.Debug("error while checking pending transactions", "error", err)
			continue
		}
		lastBlock = block
	}
}

func (tm *transactionMonitor) hasWatches() bool {
	tm.lock.Lock()
	defer tm.lock.Unlock()
	return len(tm.watchesByNonce) > 0
}

func watchStart(watches []transactionWatch) time.Time {
	if len(watches) == 0 {
		return time.Time{}
	}
	start := watches[0].start
	for _, w := range watches[1:] {
		if w.start.Before(start) {
			start = w.start
		}
	}
	return start
}

// check pending checks the given block (number) for confirmed or cancelled transactions
func (tm *transactionMonitor) checkPending(block uint64) error {
	confirmedNonces := make(map[uint64]*types.Receipt)
	var cancelledNonces []uint64
	for nonceGroup, watchMap := range tm.watchesByNonce {
		for txHash, watches := range watchMap {
			receipt, err := tm.backend.TransactionReceipt(tm.ctx, txHash)
			if err != nil {
				// wait for a few blocks to be mined before considering a transaction not existing
				transactionWatchNotFoundTimeout := 5 * tm.pollingInterval
				if errors.Is(err, ethereum.NotFound) && watchStart(watches).Before(time.Now().Add(transactionWatchNotFoundTimeout)) {
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

	for nonceGroup := range tm.watchesByNonce {
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
		for txHash, watches := range tm.watchesByNonce[nonce] {
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

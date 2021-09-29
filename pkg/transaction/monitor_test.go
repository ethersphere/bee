// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/transaction/backendsimulation"
)

func TestMonitorWatchTransaction(t *testing.T) {
	logger := logging.New(io.Discard, 0)
	txHash := common.HexToHash("0xabcd")
	nonce := uint64(10)
	sender := common.HexToAddress("0xffee")
	pollingInterval := 1 * time.Millisecond
	cancellationDepth := uint64(5)

	testTimeout := 5 * time.Second

	t.Run("single transaction confirmed", func(t *testing.T) {
		monitor := transaction.NewMonitor(
			logger,
			backendsimulation.New(
				backendsimulation.WithBlocks(
					backendsimulation.Block{
						Number: 0,
					},
					backendsimulation.Block{
						Number: 1,
						Receipts: map[common.Hash]*types.Receipt{
							txHash: {TxHash: txHash},
						},
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1,
								Account:     sender,
							}: nonce + 1,
						},
					},
				),
			),
			sender,
			pollingInterval,
			cancellationDepth,
		)

		receiptC, errC, err := monitor.WatchTransaction(txHash, nonce)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case receipt := <-receiptC:
			if receipt.TxHash != txHash {
				t.Fatal("got wrong receipt")
			}
		case err := <-errC:
			t.Fatal(err)
		case <-time.After(testTimeout):
			t.Fatal("timed out")
		}

		err = monitor.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("single transaction cancelled", func(t *testing.T) {
		monitor := transaction.NewMonitor(
			logger,
			backendsimulation.New(
				backendsimulation.WithBlocks(
					backendsimulation.Block{
						Number: 0,
					},
					backendsimulation.Block{
						Number: 1,
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1,
								Account:     sender,
							}: nonce + 1,
						},
					},
					backendsimulation.Block{
						Number: 1 + cancellationDepth,
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1 + cancellationDepth,
								Account:     sender,
							}: nonce + 1,
						},
					},
				),
			),
			sender,
			pollingInterval,
			cancellationDepth,
		)

		receiptC, errC, err := monitor.WatchTransaction(txHash, nonce)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-receiptC:
			t.Fatal("got receipt")
		case err := <-errC:
			if !errors.Is(err, transaction.ErrTransactionCancelled) {
				t.Fatalf("got wrong error. wanted %v, got %v", transaction.ErrTransactionCancelled, err)
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out")
		}

		err = monitor.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("multiple transactions mixed", func(t *testing.T) {
		txHash2 := common.HexToHash("bbbb")
		txHash3 := common.HexToHash("cccc")

		monitor := transaction.NewMonitor(
			logger,
			backendsimulation.New(
				backendsimulation.WithBlocks(
					backendsimulation.Block{
						Number: 0,
					},
					backendsimulation.Block{
						Number: 1,
						Receipts: map[common.Hash]*types.Receipt{
							txHash: {TxHash: txHash},
						},
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1,
								Account:     sender,
							}: nonce + 1,
						},
					},
					backendsimulation.Block{
						Number: 2,
						Receipts: map[common.Hash]*types.Receipt{
							txHash: {TxHash: txHash},
						},
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 2,
								Account:     sender,
							}: nonce + 2,
						},
					},
					backendsimulation.Block{
						Number: 3,
						Receipts: map[common.Hash]*types.Receipt{
							txHash:  {TxHash: txHash},
							txHash3: {TxHash: txHash3},
						},
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 3,
								Account:     sender,
							}: nonce + 4,
						},
					},
					backendsimulation.Block{
						Number: 3 + cancellationDepth,
						Receipts: map[common.Hash]*types.Receipt{
							txHash:  {TxHash: txHash},
							txHash3: {TxHash: txHash3},
						},
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 3 + cancellationDepth,
								Account:     sender,
							}: nonce + 4,
						},
					},
				),
			),
			sender,
			pollingInterval,
			cancellationDepth,
		)

		receiptC, errC, err := monitor.WatchTransaction(txHash, nonce)
		if err != nil {
			t.Fatal(err)
		}

		receiptC2, errC2, err := monitor.WatchTransaction(txHash2, nonce)
		if err != nil {
			t.Fatal(err)
		}

		receiptC3, errC3, err := monitor.WatchTransaction(txHash3, nonce)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case receipt := <-receiptC:
			if receipt.TxHash != txHash {
				t.Fatal("got wrong receipt")
			}
		case err := <-errC:
			t.Fatalf("got wrong error. wanted %v, got %v", transaction.ErrTransactionCancelled, err)
		case <-time.After(testTimeout):
			t.Fatal("timed out")
		}

		select {
		case <-receiptC2:
			t.Fatal("got receipt")
		case err := <-errC2:
			if !errors.Is(err, transaction.ErrTransactionCancelled) {
				t.Fatalf("got wrong error. wanted %v, got %v", transaction.ErrTransactionCancelled, err)
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out")
		}

		select {
		case receipt := <-receiptC3:
			if receipt.TxHash != txHash3 {
				t.Fatal("got wrong receipt")
			}
		case err := <-errC3:
			t.Fatal(err)
		case <-time.After(testTimeout):
			t.Fatal("timed out")
		}

		err = monitor.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("single transaction no confirm", func(t *testing.T) {
		txHash2 := common.HexToHash("bbbb")
		monitor := transaction.NewMonitor(
			logger,
			backendsimulation.New(
				backendsimulation.WithBlocks(
					backendsimulation.Block{
						Number: 0,
					},
					backendsimulation.Block{
						Number: 1,
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1,
								Account:     sender,
							}: nonce,
						},
					},
					backendsimulation.Block{
						Number: 1 + cancellationDepth,
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1,
								Account:     sender,
							}: nonce,
							{
								BlockNumber: 1 + cancellationDepth,
								Account:     sender,
							}: nonce + 1,
						},
					},
					backendsimulation.Block{
						Number: 1 + cancellationDepth + 1,
						Receipts: map[common.Hash]*types.Receipt{
							txHash2: {TxHash: txHash2},
						},
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1 + cancellationDepth + 1,
								Account:     sender,
							}: nonce + 1,
						},
					},
				),
			),
			sender,
			pollingInterval,
			cancellationDepth,
		)

		receiptC, errC, err := monitor.WatchTransaction(txHash, nonce)
		if err != nil {
			t.Fatal(err)
		}

		receiptC2, errC2, err := monitor.WatchTransaction(txHash2, nonce)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-receiptC:
			t.Fatal("got receipt")
		case err := <-errC:
			t.Fatal(err)
		case <-receiptC2:
		case err := <-errC2:
			t.Fatal(err)
		case <-time.After(1 * time.Second):
			t.Fatal("timeout")
		}

		err = monitor.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("shutdown while waiting", func(t *testing.T) {
		monitor := transaction.NewMonitor(
			logger,
			backendsimulation.New(
				backendsimulation.WithBlocks(
					backendsimulation.Block{
						Number: 0,
					},
					backendsimulation.Block{
						Number: 1,
						NoncesAt: map[backendsimulation.AccountAtKey]uint64{
							{
								BlockNumber: 1,
								Account:     sender,
							}: nonce + 1,
						},
					},
				),
			),
			sender,
			pollingInterval,
			cancellationDepth,
		)

		receiptC, errC, err := monitor.WatchTransaction(txHash, nonce)
		if err != nil {
			t.Fatal(err)
		}

		err = monitor.Close()
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-receiptC:
			t.Fatal("got receipt")
		case err := <-errC:
			if !errors.Is(err, transaction.ErrMonitorClosed) {
				t.Fatalf("got wrong error. wanted %v, got %v", transaction.ErrMonitorClosed, err)
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out")
		}
	})

}

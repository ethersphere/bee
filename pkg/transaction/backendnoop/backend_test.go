// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package backendnoop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendnoop"
)

func TestBackend(t *testing.T) {
	chainID := int64(1337)
	backend := backendnoop.New(chainID)

	ctx := context.Background()

	// Test ChainID
	id, err := backend.ChainID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id.Int64() != chainID {
		t.Fatalf("expected chain ID %d, got %d", chainID, id.Int64())
	}

	// Test BalanceAt returns ErrChainDisabled
	balance, err := backend.BalanceAt(ctx, common.Address{}, nil)
	if !errors.Is(err, postagecontract.ErrChainDisabled) {
		t.Fatalf("expected ErrChainDisabled, got %v", err)
	}
	if balance != nil {
		t.Fatalf("expected nil balance, got %v", balance)
	}

	// Test BlockNumber
	blockNum, err := backend.BlockNumber(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if blockNum != 4 {
		t.Fatalf("expected block number 4, got %d", blockNum)
	}

	// Test HeaderByNumber
	header, err := backend.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if header == nil {
		t.Fatal("expected header, got nil")
	}

	// Test TransactionReceipt
	receipt, err := backend.TransactionReceipt(ctx, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.BlockNumber.Int64() != 1 {
		t.Fatalf("expected receipt block number 1, got %d", receipt.BlockNumber.Int64())
	}

	// Test CallContract returns error
	result, err := backend.CallContract(ctx, ethereum.CallMsg{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Fatal("expected nil result, got data")
	}

	// Test Close (should not panic)
	backend.Close()
}

func TestBackendPanics(t *testing.T) {
	backend := backendnoop.New(1337)
	ctx := context.Background()

	// Test that panic methods actually panic
	panics := []func(){
		func() { _, _ = backend.PendingNonceAt(ctx, common.Address{}) },
		func() { _, _, _ = backend.SuggestedFeeAndTip(ctx, nil, 0) },
		func() { _, _ = backend.SuggestGasTipCap(ctx) },
		func() { _, _ = backend.EstimateGas(ctx, ethereum.CallMsg{}) },
		func() { _ = backend.SendTransaction(ctx, nil) },
		func() { _, _, _ = backend.TransactionByHash(ctx, common.Hash{}) },
		func() { _, _ = backend.NonceAt(ctx, common.Address{}, nil) },
		func() { _, _ = backend.FilterLogs(ctx, ethereum.FilterQuery{}) },
	}

	for i, panicFunc := range panics {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for function %d, but didn't panic", i)
				}
			}()
			panicFunc()
		}()
	}
}

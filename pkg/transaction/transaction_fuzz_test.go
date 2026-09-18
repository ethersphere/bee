// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	backendmock "github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/monitormock"
	abiutil "github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// FuzzUnwrapABIError tests that UnwrapABIError never panics on arbitrary RPC revert
// error payloads returned by external RPC providers (SLICE-03).
// Specifically, short buffers (< 4 bytes, e.g. "0x", "0x12", "0x1234") must not
// panic when checking the 4-byte ABI selector buf[:4].
func FuzzUnwrapABIError(f *testing.F) {
	// Valid standard revert (Error(string)) selector + payload.
	f.Add("0x08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000b6572726f72206d73670000000000000000000000000000000000000000000000")
	// Valid custom error payload.
	f.Add("0xcf4791810000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000006f")
	// Benign edges and crash triggers (< 4 bytes).
	f.Add("")
	f.Add("0x")
	f.Add("0x12")
	f.Add("0x1234")
	f.Add("0x123456")
	f.Add("not-hex")

	contractABI := abiutil.MustParseABI(`[{"inputs":[{"internalType":"uint256","name":"available","type":"uint256"},{"internalType":"uint256","name":"required","type":"uint256"}],"name":"InsufficientBalance","type":"error"},{"inputs":[{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],"name":"transfer","outputs":[],"stateMutability":"nonpayable","type":"function"}]`)

	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xbbbddd")
	chainID := big.NewInt(5)

	signedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID: chainID,
		Nonce:   1,
		To:      &recipient,
		Value:   big.NewInt(0),
	})

	request := &transaction.TxRequest{
		To:   &recipient,
		Data: []byte{},
	}

	f.Fuzz(func(t *testing.T, rpcErrData string) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		rpcAPIErr := &rpcAPIError{
			code: 3,
			msg:  "execution reverted",
			err:  rpcErrData,
		}

		transactionService, err := transaction.NewService(
			log.Noop,
			sender,
			backendmock.New(
				backendmock.WithCallContractFunc(func(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
					return nil, rpcAPIErr
				}),
			),
			signerMockForTransaction(t, signedTx, recipient, chainID),
			storemock.NewStateStore(),
			chainID,
			monitormock.New(),
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer testutil.CleanupCloser(t, transactionService)

		originErr := errors.New("origin error")

		// Primary invariant: UnwrapABIError must never panic on arbitrary rpcErrData.
		wrappedErr := transactionService.UnwrapABIError(ctx, request, originErr, contractABI.Errors)
		if wrappedErr == nil {
			t.Fatal("expected non-nil error")
		}
		if !errors.Is(wrappedErr, originErr) {
			t.Fatalf("expected wrapped error to wrap originErr: got %v", wrappedErr)
		}
	})
}

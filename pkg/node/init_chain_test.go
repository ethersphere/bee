// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	cryptomock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	statestoremock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
)

// These tests cover the chainEnabled=false path through InitChain. The
// chainEnabled=true path needs a real RPC server to exercise and is covered by
// integration tests. The points pinned here are:
//
//   - the no-op backend wires through and the function returns a well-formed
//     tuple with the requested chainID;
//   - chainID == -1 is a sentinel that skips the mismatch check, so any backend
//     chain id is accepted;
//   - a signer that fails to expose its Ethereum address surfaces a
//     "blockchain address: …" error before the transaction service is built.

const testChainID = int64(12345)

func TestInitChain_ChainDisabledHappyPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wantEth := common.HexToAddress("0x1111111111111111111111111111111111111111")
	signer := cryptomock.New(cryptomock.WithEthereumAddressFunc(func() (common.Address, error) {
		return wantEth, nil
	}))

	backend, ethAddr, gotChainID, monitor, txService, err := node.InitChain(
		ctx,
		log.Noop,
		statestoremock.NewStateStore(),
		testChainID,
		signer,
		100*time.Millisecond,
		false, // chainEnabled
		0,
		0,
		node.BlockchainRPCConfig{},
		0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() {
		if backend != nil {
			backend.Close()
		}
		if monitor != nil {
			if err := monitor.Close(); err != nil {
				t.Errorf("monitor close: %v", err)
			}
		}
		if txService != nil {
			if err := txService.Close(); err != nil {
				t.Errorf("txService close: %v", err)
			}
		}
	})

	if ethAddr != wantEth {
		t.Errorf("eth address mismatch: got %s, want %s", ethAddr, wantEth)
	}
	if gotChainID != testChainID {
		t.Errorf("chain id mismatch: got %d, want %d", gotChainID, testChainID)
	}
	if backend == nil {
		t.Error("backend must not be nil")
	}
	if monitor == nil {
		t.Error("transaction monitor must not be nil")
	}
	if txService == nil {
		t.Error("transaction service must not be nil")
	}
}

func TestInitChain_ChainIDSentinelAcceptsAnyBackendChainID(t *testing.T) {
	t.Parallel()

	signer := cryptomock.New(cryptomock.WithEthereumAddressFunc(func() (common.Address, error) {
		return common.Address{}, nil
	}))

	// chainID == -1 tells InitChain to skip the equality check. With
	// chainEnabled=false the no-op backend returns -1 too, so any value
	// passes — but the documented sentinel behavior is what we pin here.
	backend, _, gotChainID, monitor, txService, err := node.InitChain(
		context.Background(),
		log.Noop,
		statestoremock.NewStateStore(),
		-1,
		signer,
		100*time.Millisecond,
		false,
		0, 0,
		node.BlockchainRPCConfig{},
		0,
	)
	if err != nil {
		t.Fatalf("InitChain with chainID=-1 returned %v", err)
	}
	t.Cleanup(func() {
		if backend != nil {
			backend.Close()
		}
		if monitor != nil {
			_ = monitor.Close()
		}
		if txService != nil {
			_ = txService.Close()
		}
	})

	if gotChainID != -1 {
		t.Errorf("backend chain id: got %d, want -1 (the sentinel passed through)", gotChainID)
	}
}

func TestInitChain_SignerError_IsWrappedAsBlockchainAddress(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("signer offline")
	signer := cryptomock.New(cryptomock.WithEthereumAddressFunc(func() (common.Address, error) {
		return common.Address{}, wantErr
	}))

	//nolint:dogsled // InitChain has six returns; the test only needs err.
	_, _, _, _, _, err := node.InitChain(
		context.Background(),
		log.Noop,
		statestoremock.NewStateStore(),
		testChainID,
		signer,
		100*time.Millisecond,
		false,
		0, 0,
		node.BlockchainRPCConfig{},
		0,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped signer error %v, got %v", wantErr, err)
	}
	if !strings.Contains(err.Error(), "blockchain address") {
		t.Fatalf("expected error to start with %q, got %q", "blockchain address", err.Error())
	}
}

// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsync_test

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/chainsync"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
)

func TestProve(t *testing.T) {
	var (
		expHash     = "9de2787d1d80a6164f4bc6359d9017131cbc14402ee0704bff0c6d691701c1dc"
		mtx         sync.Mutex
		calledBlock string
		trxBlock    = common.HexToHash("0x2")

		nextBlockHeader = &types.Header{
			ParentHash: trxBlock,
		}
	)
	headerByNum := backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
		mtx.Lock()
		calledBlock = number.String()
		mtx.Unlock()
		return nextBlockHeader, nil
	})

	mock := backendmock.New(headerByNum)
	server, err := chainsync.New(nil, mock)
	if err != nil {
		t.Fatal(err)
	}

	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	client, err := chainsync.New(recorder, mock)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	hash, err := client.Prove(context.Background(), addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	if hex.EncodeToString(hash) != expHash {
		t.Fatalf("want '%s' got '%s'", expHash, hash)
	}

	mtx.Lock()
	if calledBlock != "1" {
		t.Fatalf("expected call block 1 got %s", calledBlock)
	}
	mtx.Unlock()
}

func TestProveErr(t *testing.T) {
	headerByNum := backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
		return nil, errors.New("some error")
	})

	mock := backendmock.New(headerByNum)
	server, err := chainsync.New(nil, mock)
	if err != nil {
		t.Fatal(err)
	}

	recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

	client, err := chainsync.New(recorder, mock)
	if err != nil {
		t.Fatal(err)
	}

	addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	_, err = client.Prove(context.Background(), addr, 1)
	if err == nil {
		t.Fatal("expected error but got none")
	}
}

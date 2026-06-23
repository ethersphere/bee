// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot"
	mocks "github.com/ethersphere/bee/v2/pkg/statestore/mock"
)

// TestSnapshotHandoffNoGap is the regression guard for the snapshot->RPC handoff
// gap (#5495). It drives the full path the node uses: build the snapshot inputs
// with snapshot.New (a real snapshot listener over a real SnapshotLogFilterer),
// replay them via batchservice.New, then Start live sync.
//
// The replay stops a few blocks below the snapshot's max block (the listener
// trims its tip), so live sync must resume from cs.Block+1 — exactly where the
// replay stopped. On the buggy version live sync was forced to the snapshot's
// nominal tip (maxBlock+1) via ResumeBlock, skipping the trimmed blocks: this
// test fails there and passes once that override is removed. The test never names
// the ResumeBlock field, so it compiles against both versions.
func TestSnapshotHandoffNoGap(t *testing.T) {
	t.Parallel()

	const maxBlock = uint64(5000)

	// A snapshot whose newest log sits at maxBlock. The logs use a non-matching
	// address so the snapshot listener filters them out and only advances the
	// chain state via the per-page UpdateBlockNumber(to) — isolating the resume
	// point. snapshot.New is what wires the (buggy) ResumeBlock on the old code.
	logs := []types.Log{
		{BlockNumber: 10, Address: common.HexToAddress("0x1"), Topics: []common.Hash{}},
		{BlockNumber: maxBlock, Address: common.HexToAddress("0x1"), Topics: []common.Hash{}},
	}
	snap, err := snapshot.New(context.Background(), testLog, rawSnapshotGetter(gzipSnapshot(t, logs)), nil,
		common.Address{}, abi.ABI{}, time.Second, time.Minute, time.Second, 0)
	if err != nil {
		t.Fatalf("snapshot.New: %v", err)
	}

	s := mocks.NewStateStore()
	store := mock.New()
	// Valid chain state so the replay's UpdateBlockNumber can advance it.
	putChainState(t, store, &postage.ChainState{Block: 0, TotalAmount: big.NewInt(0), CurrentPrice: big.NewInt(0)})

	live := &recordingListener{}
	svc, loaded, err := batchservice.New(context.Background(), s, store, testLog, live, nil, nil, nil, snap, false)
	if err != nil {
		t.Fatalf("batchservice.New: %v", err)
	}
	if !loaded {
		t.Fatal("expected snapshot to be loaded")
	}

	cs := store.GetChainState()
	if cs.Block >= maxBlock {
		t.Fatalf("replay reached %d, expected to stop below the snapshot max block %d", cs.Block, maxBlock)
	}

	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatalf("start: %v", err)
	}

	// The gap regression guard: live sync must resume from where the replay
	// actually stopped (cs.Block+1), not from the snapshot's nominal tip.
	if live.from != cs.Block+1 {
		t.Fatalf("live sync resumed from %d; must resume from cs.Block+1 = %d (resuming higher skips the snapshot's trimmed tail — see #5495)", live.from, cs.Block+1)
	}
}

func gzipSnapshot(t *testing.T, logs []types.Log) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := json.NewEncoder(gz)
	for _, l := range logs {
		if err := enc.Encode(l); err != nil {
			t.Fatalf("encode log: %v", err)
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

type rawSnapshotGetter []byte

func (g rawSnapshotGetter) GetBatchSnapshot() []byte { return g }

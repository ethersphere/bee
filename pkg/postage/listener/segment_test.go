// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener_test

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// recordingFilterer records every FilterQuery and serves logs whose address
// and block fall inside the query.
type recordingFilterer struct {
	blockNumber uint64
	logs        []types.Log

	mu      sync.Mutex
	queries []ethereum.FilterQuery
}

func (f *recordingFilterer) BlockNumber(context.Context) (uint64, error) {
	return f.blockNumber, nil
}

func (f *recordingFilterer) FilterLogs(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.mu.Unlock()
	var out []types.Log
	for _, e := range f.logs {
		if e.BlockNumber < q.FromBlock.Uint64() || e.BlockNumber > q.ToBlock.Uint64() {
			continue
		}
		for _, a := range q.Addresses {
			if a == e.Address {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func TestListenerSegmentSwitch(t *testing.T) {
	t.Parallel()

	const boundary = 50
	addrA := common.HexToAddress("aaaa")
	addrB := common.HexToAddress("bbbb")

	createA := createArgs{id: hash[:], owner: addr[:], amount: big.NewInt(1), normalisedAmount: big.NewInt(1), depth: 20}
	createB := createArgs{id: hash[:], owner: addr[:], amount: big.NewInt(2), normalisedAmount: big.NewInt(2), depth: 21}
	logA := createA.toLog(10)
	logA.Address = addrA
	logB := createB.toLog(60)
	logB.Address = addrB

	f := &recordingFilterer{
		blockNumber: 120 + uint64(listener.TailSize), // sync target 120
		logs:        []types.Log{logA, logB},
	}

	l, err := listener.NewSegmented(nil, log.Noop, f, []listener.Segment{
		{Address: addrA, ABI: postageStampContractABI, FromBlock: 0},
		{Address: addrB, ABI: postageStampContractABI, FromBlock: boundary},
	}, 1, stallingTimeout, backoffTime, 1000)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, l)

	ev := &updater{eventC: make(chan any, 100)}
	select {
	case err := <-l.Listen(t.Context(), 0, ev):
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for sync")
	}

	f.mu.Lock()
	queries := append([]ethereum.FilterQuery(nil), f.queries...)
	f.mu.Unlock()

	var sawA, sawB bool
	for _, q := range queries {
		from, to := q.FromBlock.Uint64(), q.ToBlock.Uint64()
		if from < boundary && to >= boundary {
			t.Fatalf("query [%d,%d] spans segment boundary", from, to)
		}
		if len(q.Addresses) != 1 {
			t.Fatalf("query [%d,%d] has %d addresses", from, to, len(q.Addresses))
		}
		switch {
		case to < boundary:
			sawA = true
			if q.Addresses[0] != addrA {
				t.Fatalf("query [%d,%d] uses %s, want A", from, to, q.Addresses[0])
			}
		case from >= boundary:
			sawB = true
			if q.Addresses[0] != addrB {
				t.Fatalf("query [%d,%d] uses %s, want B", from, to, q.Addresses[0])
			}
		}
	}
	if !sawA || !sawB {
		t.Fatalf("expected queries on both segments, got %d queries", len(queries))
	}

	var creates []createArgs
	for len(ev.eventC) > 0 {
		if c, ok := (<-ev.eventC).(createArgs); ok {
			creates = append(creates, c)
		}
	}
	if len(creates) != 2 {
		t.Fatalf("got %d create events, want 2", len(creates))
	}
	creates[0].compareF(t, createA)
	creates[1].compareF(t, createB)
}

func TestNewSegmentedInvalid(t *testing.T) {
	t.Parallel()

	tests := map[string][]listener.Segment{
		"empty":          nil,
		"first not 0":    {{FromBlock: 1}},
		"equal":          {{FromBlock: 0}, {FromBlock: 0}},
		"non-increasing": {{FromBlock: 0}, {FromBlock: 10}, {FromBlock: 5}},
	}
	for name, segs := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := listener.NewSegmented(nil, log.Noop, &recordingFilterer{}, segs, 1, stallingTimeout, backoffTime, 0)
			if !errors.Is(err, listener.ErrInvalidSegments) {
				t.Fatalf("got %v, want ErrInvalidSegments", err)
			}
		})
	}
}

func TestListenerSegmentPaused(t *testing.T) {
	t.Parallel()

	const boundary = 50
	addrA := common.HexToAddress("aaaa")
	addrB := common.HexToAddress("bbbb")

	pausedLog := func(a common.Address, block uint64) types.Log {
		return types.Log{
			Address:     a,
			BlockNumber: block,
			Topics:      []common.Hash{postageStampContractABI.Events["Paused"].ID},
			Data:        make([]byte, 32),
		}
	}
	segments := []listener.Segment{
		{Address: addrA, ABI: postageStampContractABI, FromBlock: 0},
		{Address: addrB, ABI: postageStampContractABI, FromBlock: boundary},
	}

	listen := func(t *testing.T, logs []types.Log, ev *updater) error {
		t.Helper()
		f := &recordingFilterer{
			blockNumber: 120 + uint64(listener.TailSize),
			logs:        logs,
		}
		l, err := listener.NewSegmented(nil, log.Noop, f, segments, 1, stallingTimeout, backoffTime, 1000)
		if err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, l)
		select {
		case err := <-l.Listen(t.Context(), 0, ev):
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for sync")
		}
		return nil
	}

	t.Run("superseded segment paused is ignored", func(t *testing.T) {
		t.Parallel()

		createB := createArgs{id: hash[:], owner: addr[:], amount: big.NewInt(2), normalisedAmount: big.NewInt(2), depth: 21}
		logB := createB.toLog(60)
		logB.Address = addrB

		ev := &updater{eventC: make(chan any, 100)}
		if err := listen(t, []types.Log{pausedLog(addrA, 10), logB}, ev); err != nil {
			t.Fatalf("got %v, want sync to continue", err)
		}

		var creates []createArgs
		for len(ev.eventC) > 0 {
			if c, ok := (<-ev.eventC).(createArgs); ok {
				creates = append(creates, c)
			}
		}
		if len(creates) != 1 {
			t.Fatalf("got %d create events, want 1", len(creates))
		}
		creates[0].compareF(t, createB)
	})

	t.Run("last segment paused stops sync", func(t *testing.T) {
		t.Parallel()

		ev := &updater{eventC: make(chan any, 100)}
		err := listen(t, []types.Log{pausedLog(addrB, 60)}, ev)
		if !errors.Is(err, listener.ErrPostagePaused) {
			t.Fatalf("got %v, want %v", err, listener.ErrPostagePaused)
		}
	})
}

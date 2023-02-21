// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocker_test

import (
	"os"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/ethersphere/bee/pkg/blocker"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

const (
	sequencerResolution = time.Millisecond
	flagTime            = sequencerResolution * 25
	blockTime           = time.Second
)

var (
	addr   = test.RandomAddress()
	logger = log.Noop
)

func TestMain(m *testing.M) {
	defer func(resolution time.Duration) {
		*blocker.SequencerResolution = resolution
	}(*blocker.SequencerResolution)
	*blocker.SequencerResolution = sequencerResolution

	os.Exit(m.Run())

	goleak.VerifyTestMain(m)
}

func TestBlocksAfterFlagTimeout(t *testing.T) {
	t.Parallel()

	blockedC := make(chan swarm.Address, 10)

	mock := mockBlockLister(func(a swarm.Address, d time.Duration, r string) error {
		blockedC <- a

		if d != blockTime {
			t.Fatalf("block time: want %v, got %v", blockTime, d)
		}

		return nil
	})

	b := blocker.New(mock, flagTime, blockTime, time.Millisecond, nil, logger)
	testutil.CleanupCloser(t, b)

	// Flagging address shouldn't block it immediately
	b.Flag(addr)
	if len(blockedC) != 0 {
		t.Fatal("blocker did not wait flag duration")
	}

	time.Sleep(flagTime / 2)
	b.Flag(addr) // check thats this flag call does not overide previous call
	if len(blockedC) != 0 {
		t.Fatal("blocker did not wait flag duration")
	}

	// Suspending current goroutine and expect that in this interval
	// block listener was called to block flagged address
	time.Sleep(flagTime * 3)

	if a := <-blockedC; !a.Equal(addr) {
		t.Fatalf("expecting flagged address to be blocked")
	}
	if len(blockedC) != 0 {
		t.Fatalf("address should only be blocked once")
	}
}

func TestUnflagBeforeBlock(t *testing.T) {
	t.Parallel()

	mock := mockBlockLister(func(a swarm.Address, d time.Duration, r string) error {
		t.Fatalf("address should not be blocked")

		return nil
	})

	b := blocker.New(mock, flagTime, blockTime, time.Millisecond, nil, logger)
	testutil.CleanupCloser(t, b)

	// Flagging address shouldn't block it imidietly
	b.Flag(addr)

	time.Sleep(flagTime / 2)
	b.Flag(addr) // check thats this flag call does not overide previous call

	b.Unflag(addr)

	// Suspending current goroutine and expect that in this interval
	// block listener was not called to block flagged address
	time.Sleep(flagTime * 3)
}

func TestPruneBeforeBlock(t *testing.T) {
	t.Parallel()

	mock := mockBlockLister(func(a swarm.Address, d time.Duration, r string) error {
		t.Fatalf("address should not be blocked")

		return nil
	})

	b := blocker.New(mock, flagTime, blockTime, time.Millisecond, nil, logger)
	testutil.CleanupCloser(t, b)

	// Flagging address shouldn't block it imidietly
	b.Flag(addr)

	time.Sleep(flagTime / 2)

	// communicate that we have seen no peers, resulting in the peer being removed
	b.PruneUnseen([]swarm.Address{})

	// Suspending current goroutine expect that in this interval
	// block listener was not called to block flagged address
	time.Sleep(flagTime * 3)
}

type blocklister struct {
	blocklistFunc func(swarm.Address, time.Duration, string) error
}

func mockBlockLister(f func(swarm.Address, time.Duration, string) error) *blocklister {
	return &blocklister{
		blocklistFunc: f,
	}
}

func (b *blocklister) Blocklist(addr swarm.Address, t time.Duration, r string) error {
	return b.blocklistFunc(addr, t, r)
}

// NetworkStatus implements p2p.NetworkStatuser interface.
// It always returns p2p.NetworkStatusAvailable.
func (b *blocklister) NetworkStatus() p2p.NetworkStatus {
	return p2p.NetworkStatusAvailable
}

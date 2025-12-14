// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
)

func dbTestOps(baseAddr swarm.Address, reserveCapacity int, bs postage.Storer, radiusSetter topology.SetStorageRadiuser, reserveWakeUpTime time.Duration) *Options {
	opts := DefaultOptions()
	if radiusSetter == nil {
		radiusSetter = kademlia.NewTopologyDriver()
	}

	if bs == nil {
		bs = batchstore.New()
	}

	opts.Address = baseAddr
	opts.RadiusSetter = radiusSetter
	opts.ReserveCapacity = reserveCapacity
	opts.Batchstore = bs
	opts.ReserveWakeUpDuration = reserveWakeUpTime
	opts.Logger = log.Noop

	return opts
}

type testRetrieval struct {
	fn func(swarm.Address) (swarm.Chunk, error)
}

func (t *testRetrieval) RetrieveChunk(_ context.Context, address swarm.Address, _ swarm.Address) (swarm.Chunk, error) {
	return t.fn(address)
}

func TestPinIntegrity_Repair(t *testing.T) {

	opts := dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)
	db, err := New(context.Background(), os.TempDir(), opts)
	if err != nil {
		t.Fatal(err)
	}

	chunks := chunktesting.GenerateTestRandomChunks(10)
	session, err := db.NewCollection(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	for _, ch := range chunks {
		err := session.Put(context.TODO(), ch)
		if err != nil {
			t.Fatal(err)
		}
	}

	pin := chunks[0].Address()
	err = session.Done(pin)
	if err != nil {
		t.Fatal(err)
	}

	// no repair needed
	count := 0
	res := make(chan RepairPinResult)
	db.pinIntegrity.Repair(context.Background(), log.Noop, pin.String(), db, res)
	for range res {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 repairs, got %d", count)
	}

	// repair needed
	count = 0
	err = db.storage.Run(context.Background(), func(s transaction.Store) error {
		return s.ChunkStore().Delete(context.Background(), chunks[1].Address())
	})
	if err != nil {
		t.Fatal(err)
	}

	res = make(chan RepairPinResult)
	db.SetRetrievalService(&testRetrieval{fn: func(address swarm.Address) (swarm.Chunk, error) {
		for _, ch := range chunks {
			if ch.Address().Equal(address) {
				return ch, nil
			}
		}
		return nil, storage.ErrNotFound
	}})
	go db.pinIntegrity.Repair(context.Background(), log.Noop, pin.String(), db, res)
	for range res {
		count++
	}

	if count != 1 {
		t.Fatalf("expected 1 repair, got %d", count)
	}

	out := make(chan PinStat)
	corrupted := make(chan CorruptedPinChunk)
	go db.pinIntegrity.Check(context.Background(), log.Noop, pin.String(), out, corrupted)
	go syncutil.Drain(corrupted)

	v := <-out
	if !v.Ref.Equal(pin) {
		t.Fatalf("expected pin %s, got %s", pin, v.Ref)
	}
	if v.Total != 10 {
		t.Fatalf("expected total 10, got %d", v.Total)
	}
	if v.Missing != 0 {
		t.Fatalf("expected missing 0, got %d", v.Missing)
	}
	if v.Invalid != 0 {
		t.Fatalf("expected invalid 0, got %d", v.Invalid)
	}
}

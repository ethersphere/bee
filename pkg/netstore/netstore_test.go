// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netstore_test

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

var testChunk = chunktesting.GenerateTestRandomChunk()
var chunkStamp = postagetesting.MustNewStamp()

// TestNetstoreRetrieval verifies that a chunk is asked from the network whenever
// it is not found locally
func TestNetstoreRetrieval(t *testing.T) {
	retrieve, store, nstore := newRetrievingNetstore(t, noopValidStamp)
	addr := testChunk.Address()
	_, err := nstore.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !retrieve.called {
		t.Fatal("retrieve request not issued")
	}
	if retrieve.callCount != 1 {
		t.Fatalf("call count %d", retrieve.callCount)
	}
	if !retrieve.addr.Equal(addr) {
		t.Fatalf("addresses not equal. got %s want %s", retrieve.addr, addr)
	}

	// store should have the chunk once the background PUT is complete
	d := waitAndGetChunk(t, store, addr, storage.ModeGetRequest)

	if !bytes.Equal(d.Data(), testChunk.Data()) {
		t.Fatal("chunk data not equal to expected data")
	}

	// check that the second call does not result in another retrieve request
	d, err = nstore.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}

	if retrieve.callCount != 1 {
		t.Fatalf("call count %d", retrieve.callCount)
	}
	if !bytes.Equal(d.Data(), testChunk.Data()) {
		t.Fatal("chunk data not equal to expected data")
	}

}

// TestNetstoreNoRetrieval verifies that a chunk is not requested from the network
// whenever it is found locally.
func TestNetstoreNoRetrieval(t *testing.T) {
	retrieve, store, nstore := newRetrievingNetstore(t, noopValidStamp)
	addr := testChunk.Address()

	// store should have the chunk in advance
	_, err := store.Put(context.Background(), storage.ModePutUpload, testChunk)
	if err != nil {
		t.Fatal(err)
	}

	c, err := nstore.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}
	if retrieve.called {
		t.Fatal("retrieve request issued but shouldn't")
	}
	if retrieve.callCount != 0 {
		t.Fatalf("call count %d", retrieve.callCount)
	}
	if !bytes.Equal(c.Data(), testChunk.Data()) {
		t.Fatal("chunk data mismatch")
	}
}

func TestInvalidChunkNetstoreRetrieval(t *testing.T) {
	retrieve, store, nstore := newRetrievingNetstore(t, noopValidStamp)

	invalidChunk := swarm.NewChunk(testChunk.Address(), []byte("deadbeef"))
	// store invalid chunk, i.e. hash doesnt match the data to simulate corruption
	_, err := store.Put(context.Background(), storage.ModePutUpload, invalidChunk)
	if err != nil {
		t.Fatal(err)
	}

	addr := testChunk.Address()
	_, err = nstore.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !retrieve.called {
		t.Fatal("retrieve request not issued")
	}
	if retrieve.callCount != 1 {
		t.Fatalf("call count %d", retrieve.callCount)
	}
	if !retrieve.addr.Equal(addr) {
		t.Fatalf("addresses not equal. got %s want %s", retrieve.addr, addr)
	}

	// store should have the chunk once the background PUT is complete
	d := waitAndGetChunk(t, store, addr, storage.ModeGetRequest)

	if !bytes.Equal(d.Data(), testChunk.Data()) {
		t.Fatal("chunk data not equal to expected data")
	}

	// check that the second call does not result in another retrieve request
	d, err = nstore.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}

	if retrieve.callCount != 1 {
		t.Fatalf("call count %d", retrieve.callCount)
	}
	if !bytes.Equal(d.Data(), testChunk.Data()) {
		t.Fatal("chunk data not equal to expected data")
	}
}

func TestInvalidPostageStamp(t *testing.T) {
	f := func(c swarm.Chunk, _ []byte) (swarm.Chunk, error) {
		return nil, errors.New("invalid postage stamp")
	}
	retrieve, store, nstore := newRetrievingNetstore(t, f)
	addr := testChunk.Address()
	_, err := nstore.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !retrieve.called {
		t.Fatal("retrieve request not issued")
	}
	if retrieve.callCount != 1 {
		t.Fatalf("call count %d", retrieve.callCount)
	}
	if !retrieve.addr.Equal(addr) {
		t.Fatalf("addresses not equal. got %s want %s", retrieve.addr, addr)
	}

	// store should have the chunk once the background PUT is complete
	d := waitAndGetChunk(t, store, addr, storage.ModeGetRequest)

	if !bytes.Equal(d.Data(), testChunk.Data()) {
		t.Fatal("chunk data not equal to expected data")
	}

	if mode := store.GetModePut(addr); mode != storage.ModePutRequestCache {
		t.Fatalf("wanted ModePutRequestCache but got %s", mode)
	}

	// check that the second call does not result in another retrieve request
	d, err = nstore.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}

	if retrieve.callCount != 1 {
		t.Fatalf("call count %d", retrieve.callCount)
	}
	if !bytes.Equal(d.Data(), testChunk.Data()) {
		t.Fatal("chunk data not equal to expected data")
	}
}

func waitAndGetChunk(t *testing.T, store storage.Storer, addr swarm.Address, mode storage.ModeGet) swarm.Chunk {
	t.Helper()

	start := time.Now()
	for {
		time.Sleep(time.Millisecond * 10)

		d, err := store.Get(context.Background(), mode, addr)
		if err != nil {
			if time.Since(start) > 3*time.Second {
				t.Fatal("waited 3 secs for background put operation", err)
			}
		} else {
			return d
		}
	}
}

// returns a mock retrieval protocol, a mock local storage and a netstore
func newRetrievingNetstore(t *testing.T, validStamp postage.ValidStampFn) (ret *retrievalMock, mockStore *mock.MockStorer, ns storage.Storer) {
	t.Helper()

	retrieve := &retrievalMock{}
	store := mock.NewStorer()
	logger := log.Noop
	ns = netstore.New(store, validStamp, retrieve, logger)
	t.Cleanup(func() {
		err := ns.Close()
		if err != nil {
			t.Fatal("failed closing netstore", err)
		}
	})
	return retrieve, store, ns
}

type retrievalMock struct {
	called    bool
	callCount int32
	failure   bool
	addr      swarm.Address
}

func (r *retrievalMock) RetrieveChunk(ctx context.Context, addr, sourceAddr swarm.Address) (chunk swarm.Chunk, err error) {
	if r.failure {
		return nil, errors.New("chunk not found")
	}
	r.called = true
	atomic.AddInt32(&r.callCount, 1)
	r.addr = addr
	return testChunk.WithStamp(chunkStamp), nil
}

var noopValidStamp = func(c swarm.Chunk, _ []byte) (swarm.Chunk, error) {
	return c, nil
}

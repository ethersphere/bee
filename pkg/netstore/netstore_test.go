// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netstore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"sync/atomic"
	"testing"
	"time"

	validatormock "github.com/ethersphere/bee/pkg/content/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

var chunkData = []byte("mockdata")

// TestNetstoreRetrieval verifies that a chunk is asked from the network whenever
// it is not found locally
func TestNetstoreRetrieval(t *testing.T) {
	retrieve, store, nstore := newRetrievingNetstore(nil)
	addr := swarm.MustParseHexAddress("000001")
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

	// store should have the chunk now
	d, err := store.Get(context.Background(), storage.ModeGetRequest, addr)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(d.Data(), chunkData) {
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
	if !bytes.Equal(d.Data(), chunkData) {
		t.Fatal("chunk data not equal to expected data")
	}

}

// TestNetstoreNoRetrieval verifies that a chunk is not requested from the network
// whenever it is found locally.
func TestNetstoreNoRetrieval(t *testing.T) {
	retrieve, store, nstore := newRetrievingNetstore(nil)
	addr := swarm.MustParseHexAddress("000001")

	// store should have the chunk in advance
	_, err := store.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(addr, chunkData))
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
	if !bytes.Equal(c.Data(), chunkData) {
		t.Fatal("chunk data mismatch")
	}
}

// TestRecovery verifies if recovery is called if it is not empty
func TestRecovery(t *testing.T) {
	hookWasCalled := make(chan bool, 1)
	rec := &mockRecovery{
		hookC: hookWasCalled,
	}

	retrieve, _, nstore := newRetrievingNetstore(rec)
	addr := swarm.MustParseHexAddress("deadbeef")
	retrieve.failure = true
	ctx := context.Background()
	ctx = sctx.SetTargets(ctx, "be, cd")

	_, err := nstore.Get(ctx, storage.ModeGetRequest, addr)
	if err != nil && !errors.Is(err, netstore.ErrRecoveryAttempt) {
		t.Fatal(err)
	}

	select {
	case <-hookWasCalled:
		break
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recovery hook was not called")
	}
}

// TestRecovery verifies that recovery function is not called if it is not set
func TestInvalidRecoveryFunction(t *testing.T) {
	retrieve, _, nstore := newRetrievingNetstore(nil)
	addr := swarm.MustParseHexAddress("deadbeef")
	retrieve.failure = true
	ctx := context.Background()
	ctx = sctx.SetTargets(ctx, "be, cd")

	_, err := nstore.Get(ctx, storage.ModeGetRequest, addr)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}
}

// TestInvalidTargets verifies that the recovery is not triggered if the target is invalid
func TestInvalidTargets(t *testing.T) {
	hookWasCalled := make(chan bool, 1)
	rec := &mockRecovery{
		hookC: hookWasCalled,
	}

	retrieve, _, nstore := newRetrievingNetstore(rec)
	addr := swarm.MustParseHexAddress("deadbeef")
	retrieve.failure = true
	ctx := context.Background()
	ctx = sctx.SetTargets(ctx, "gh")

	_, err := nstore.Get(ctx, storage.ModeGetRequest, addr)
	if err != nil && !errors.Is(err, sctx.ErrTargetPrefix) {
		t.Fatal(err)
	}
}

// TestEmptyTarget verifies that the recovery is not triggered if the target is empty string
func TestEmptyTarget(t *testing.T) {
	hookWasCalled := make(chan bool, 1)
	rec := &mockRecovery{
		hookC: hookWasCalled,
	}

	retrieve, _, nstore := newRetrievingNetstore(rec)
	addr := swarm.MustParseHexAddress("deadbeef")
	retrieve.failure = true
	ctx := context.Background()
	ctx = sctx.SetTargets(ctx, "")

	_, err := nstore.Get(ctx, storage.ModeGetRequest, addr)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}
}

// returns a mock retrieval protocol, a mock local storage and a netstore
func newRetrievingNetstore(rec *mockRecovery) (ret *retrievalMock, mockStore, ns storage.Storer) {
	retrieve := &retrievalMock{}
	store := mock.NewStorer()
	logger := logging.New(ioutil.Discard, 0)
	validator := swarm.NewChunkValidator(validatormock.NewValidator(true))

	var nstore storage.Storer
	if rec != nil {
		nstore = netstore.New(store, rec.recovery, retrieve, logger, validator)
	} else {
		nstore = netstore.New(store, nil, retrieve, logger, validator)
	}
	return retrieve, store, nstore
}

type retrievalMock struct {
	called    bool
	callCount int32
	failure   bool
	addr      swarm.Address
}

func (r *retrievalMock) RetrieveChunk(ctx context.Context, addr swarm.Address) (chunk swarm.Chunk, err error) {
	if r.failure {
		return nil, storage.ErrNotFound
	}
	r.called = true
	atomic.AddInt32(&r.callCount, 1)
	r.addr = addr
	return swarm.NewChunk(addr, chunkData), nil
}

type mockRecovery struct {
	hookC chan bool
}

// Send mocks the pss Send function
func (mr *mockRecovery) recovery(chunkAddress swarm.Address, targets trojan.Targets) error {
	mr.hookC <- true
	return nil
}

func (r *mockRecovery) RetrieveChunk(ctx context.Context, addr swarm.Address) (chunk swarm.Chunk, err error) {
	return nil, fmt.Errorf("chunk not found")
}

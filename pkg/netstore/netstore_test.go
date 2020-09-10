// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netstore_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/ioutil"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/content"
	validatormock "github.com/ethersphere/bee/pkg/content/mock"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/soc"
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

// TestRecovery verifies if the recovery hook is called if the chunk is not there in the network
// and the targets are present
func TestRecovery(t *testing.T) {
	hookWasCalled := make(chan bool, 1)
	payload := make([]byte, swarm.ChunkSize)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	rec := &mockRecovery{
		hookC:     hookWasCalled,
		sendChunk: c,
	}

	retrieve, _, nstore := newRetrievingNetstore(rec)
	netstore.SetTimeout(50 * time.Millisecond)

	retrieve.failure = true
	ctx := context.Background()
	ctx = sctx.SetTargets(ctx, "be, cd")

	rcvdChunk, err := nstore.Get(ctx, storage.ModeGetRequest, c.Address())
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-hookWasCalled:
		break
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recovery hook was not called")
	}

	if !c.Address().Equal(rcvdChunk.Address()) {
		t.Fatal()
	}
	if !bytes.Equal(c.Data(), rcvdChunk.Data()) {
		t.Fatal()
	}
}

// TestRecoveryTimeout verifies if the timout function of the netstore works properly
func TestRecoveryTimeout(t *testing.T) {
	hookWasCalled := make(chan bool, 1)
	payload := make([]byte, swarm.ChunkSize)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	rec := &mockRecovery{
		hookC:     hookWasCalled,
		sendChunk: nil,
	}

	retrieve, _, nstore := newRetrievingNetstore(rec)
	retrieve.failure = true
	ctx := context.Background()
	ctx = sctx.SetTargets(ctx, "be, cd")

	netstore.SetTimeout(50 * time.Millisecond)
	_, err = nstore.Get(ctx, storage.ModeGetRequest, c.Address())
	if err != nil && !errors.Is(err, netstore.ErrRecoveryTimeout) {
		t.Fatal(err)
	}

	select {
	case <-hookWasCalled:
		break
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recovery hook was not called")
	}
}

func TestInvalidRecoveryFunction(t *testing.T) {
	retrieve, _, nstore := newRetrievingNetstore(nil)
	addr := swarm.MustParseHexAddress("deadbeef")
	retrieve.failure = true
	ctx := context.Background()
	ctx = sctx.SetTargets(ctx, "be, cd")

	_, err := nstore.Get(ctx, storage.ModeGetRequest, addr)
	if err != nil && err.Error() != "chunk not found" {
		t.Fatal(err)
	}
}

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
		return nil, fmt.Errorf("chunk not found")
	}
	r.called = true
	atomic.AddInt32(&r.callCount, 1)
	r.addr = addr
	return swarm.NewChunk(addr, chunkData), nil
}

type mockRecovery struct {
	hookC     chan bool
	sendChunk swarm.Chunk
}

// Send mocks the pss Send function
func (mr *mockRecovery) recovery(chunkAddress swarm.Address, targets trojan.Targets, chunkC chan swarm.Chunk) (swarm.Address, error) {
	mr.hookC <- true
	if mr.sendChunk != nil {
		sch := getSocChunk(mr.sendChunk)
		chunkC <- sch
		return sch.Address(), nil
	}
	return swarm.NewAddress([]byte("deadbeef")), nil
}

func (r *mockRecovery) RetrieveChunk(ctx context.Context, addr swarm.Address) (chunk swarm.Chunk, err error) {
	return nil, fmt.Errorf("chunk not found")
}

func getSocChunk(c swarm.Chunk) swarm.Chunk {
	var dummyChunk swarm.Chunk
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		return dummyChunk
	}
	signer := crypto.NewDefaultSigner(privKey)
	id := make([]byte, 32)
	sch, err := soc.NewChunk(id, c, signer)
	if err != nil {
		return dummyChunk
	}
	return sch
}

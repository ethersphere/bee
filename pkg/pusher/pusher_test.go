// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher_test

import (
	"context"
	"errors"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/tags"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/mock"
)

// no of times to retry to see if we have received response from pushsync
var noOfRetries = 20

// Wrap the actual storer to intercept the modeSet that the pusher will call when a valid receipt is received
type Store struct {
	storage.Storer
	modeSet   map[string]storage.ModeSet
	modeSetMu *sync.Mutex
}

// Override the Set function to capture the ModeSetSyncPush
func (s Store) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) error {
	s.modeSetMu.Lock()
	defer s.modeSetMu.Unlock()
	for _, addr := range addrs {
		s.modeSet[addr.String()] = mode
	}
	return nil
}

// TestSendChunkToPushSync sends a chunk to pushsync to be sent ot its closest peer and get a receipt.
// once the receipt is got this check to see if the localstore is updated to see if the chunk is set
// as ModeSetSyncPush status.
func TestSendChunkToPushSync(t *testing.T) {
	chunk := createChunk()

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		receipt := &pushsync.Receipt{
			Address: swarm.NewAddress(chunk.Address().Bytes()),
		}
		return receipt, nil
	})
	mtag := tags.NewTags()
	tag, err := mtag.Create("name", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	tag.Address = chunk.Address()
	p, storer := createPusher(t, triggerPeer, pushSyncService, mtag, mock.WithClosestPeer(closestPeer))
	defer storer.Close()

	_, err = storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check is the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(10 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSyncPush, storer)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
}

// TestSendChunkToPushSyncWithoutTag is similar to TestSendChunkToPushSync, excep that the tags are not
// present to simulate bzz api withotu splitter condition
func TestSendChunkToPushSyncWithoutTag(t *testing.T) {
	chunk := createChunk()

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		receipt := &pushsync.Receipt{
			Address: swarm.NewAddress(chunk.Address().Bytes()),
		}
		return receipt, nil
	})
	mtag := tags.NewTags()
	_, err := mtag.Create("name", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	p, storer := createPusher(t, triggerPeer, pushSyncService, mtag, mock.WithClosestPeer(closestPeer))
	defer storer.Close()

	_, err = storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check is the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(10 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSyncPush, storer)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
}

// TestSendChunkAndReceiveInvalidReceipt sends a chunk to pushsync to be sent ot its closest peer and
// get a invalid receipt (not with the address of the chunk sent). The test makes sure that this error
// is received and the ModeSetSyncPush is not set for the chunk.
func TestSendChunkAndReceiveInvalidReceipt(t *testing.T) {
	chunk := createChunk()

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		return nil, errors.New("invalid receipt")
	})

	mtag := tags.NewTags()
	tag, err := mtag.Create("name", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	tag.Address = chunk.Address()
	p, storer := createPusher(t, triggerPeer, pushSyncService, mtag, mock.WithClosestPeer(closestPeer))
	defer storer.Close()

	_, err = storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check is the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(10 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSyncPush, storer)
		if err != nil {
			continue
		}
	}
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
	p.Close()
}

// TestSendChunkAndTimeoutinReceivingReceipt sends a chunk to pushsync to be sent ot its closest peer and
// expects a timeout to get instead of getting a receipt. The test makes sure that timeout error
// is received and the ModeSetSyncPush is not set for the chunk.
func TestSendChunkAndTimeoutinReceivingReceipt(t *testing.T) {
	chunk := createChunk()

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		// Set 10 times more than the time we wait for the test to complete so that
		// the response never reaches our testcase
		time.Sleep(1 * time.Second)
		return nil, nil
	})

	mtag := tags.NewTags()
	tag, err := mtag.Create("name", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	tag.Address = chunk.Address()
	p, storer := createPusher(t, triggerPeer, pushSyncService, mtag, mock.WithClosestPeer(closestPeer))
	defer storer.Close()
	defer p.Close()

	_, err = storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check is the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(10 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSyncPush, storer)
		if err != nil {
			continue
		}
	}
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
}

func createChunk() swarm.Chunk {
	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")
	return swarm.NewChunk(chunkAddress, chunkData)
}

func createPusher(t *testing.T, addr swarm.Address, pushSyncService pushsync.PushSyncer, tag *tags.Tags, mockOpts ...mock.Option) (*pusher.Service, *Store) {
	t.Helper()
	logger := logging.New(ioutil.Discard, 0)
	storer, err := localstore.New("", addr.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	pusherStorer := &Store{
		Storer:    storer,
		modeSet:   make(map[string]storage.ModeSet),
		modeSetMu: &sync.Mutex{},
	}
	peerSuggester := mock.NewTopologyDriver(mockOpts...)

	pusherService := pusher.New(pusher.Options{Storer: pusherStorer, Tags: tag, PushSyncer: pushSyncService, PeerSuggester: peerSuggester, Logger: logger})
	return pusherService, pusherStorer
}

func checkIfModeSet(addr swarm.Address, mode storage.ModeSet, storer *Store) error {
	var found bool
	storer.modeSetMu.Lock()
	defer storer.modeSetMu.Unlock()

	for k, v := range storer.modeSet {
		if addr.String() == k {
			found = true
			if v != mode {
				return errors.New("chunk mode is not properly set as synced")
			}
		}
	}
	if !found {
		return errors.New("Chunk not synced")
	}
	return nil
}

// To avoid timeout during race testing
// cd pkg/pusher
// go test -race -count 1000 -timeout 60m .

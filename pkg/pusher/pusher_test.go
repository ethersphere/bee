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

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/mock"
)

type Store struct {
	storage.Storer
	ModeSet   map[string]storage.ModeSet
	modeSetMu sync.Mutex
}

func (s Store) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) error {
	s.modeSetMu.Lock()
	defer s.modeSetMu.Unlock()
	for _, addr := range addrs {
		s.ModeSet[addr.String()] = mode
	}
	return s.Set(ctx, mode, addrs...)
}

func TestSendChunkToPushSync(t *testing.T) {
	chunk := createChunk(t)

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, address swarm.Address, chunk swarm.Chunk) (*pb.Receipt, error) {
		receipt := &pb.Receipt{
			Address: chunk.Address().Bytes(),
		}
		return receipt, nil
	})

	_, storer := createPusher(t, triggerPeer, pushSyncService, mock.WithClosestPeer(closestPeer))

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Give some time for chunk to be pushed and receipt to be received
	time.Sleep(10 * time.Millisecond)

	err = checkIfModeSet(t, chunk.Address(), storage.ModeSetSyncPush, storer)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendChunkAndReceiveInvalidReceipt(t *testing.T) {
	chunk := createChunk(t)

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, address swarm.Address, chunk swarm.Chunk) (*pb.Receipt, error) {
		return nil, errors.New("invalid receipt")
	})

	_, storer := createPusher(t, triggerPeer, pushSyncService, mock.WithClosestPeer(closestPeer))

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Give some time for chunk to be pushed and receipt to be received
	time.Sleep(10 * time.Millisecond)

	err = checkIfModeSet(t, chunk.Address(), storage.ModeSetSyncPush, storer)
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
}

func TestSendChunkAndTimeoutinReceivingReceipt(t *testing.T) {
	chunk := createChunk(t)

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, address swarm.Address, chunk swarm.Chunk) (*pb.Receipt, error) {
		// Set 10 times more than the time we wait for the test to complete so that
		// the response never reaches our testcase
		time.Sleep(100 * time.Millisecond)
		return nil, nil
	})

	_, storer := createPusher(t, triggerPeer, pushSyncService, mock.WithClosestPeer(closestPeer))

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Give some time for chunk to be pushed and receipt to be received
	time.Sleep(10 * time.Millisecond)

	err = checkIfModeSet(t, chunk.Address(), storage.ModeSetSyncPush, storer)
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
}

func createChunk(t *testing.T) swarm.Chunk {
	t.Helper()

	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")
	return swarm.NewChunk(chunkAddress, chunkData)
}

func createPusher(t *testing.T, addr swarm.Address, pushSyncService pushsync.PushSyncer, mockOpts ...mock.Option) (*pusher.Service, *Store) {
	t.Helper()
	logger := logging.New(ioutil.Discard, 0)
	storer, err := localstore.New("", addr.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	pusherStorer := &Store{
		Storer:  storer,
		ModeSet: make(map[string]storage.ModeSet),
	}
	peerSuggester := mock.NewTopologyDriver(mockOpts...)
	pusherService := pusher.New(pusher.Options{Storer: pusherStorer, PushSyncer: pushSyncService, PeerSuggester: peerSuggester, Logger: logger})
	return pusherService, pusherStorer
}

func checkIfModeSet(t *testing.T, addr swarm.Address, mode storage.ModeSet, storer *Store) error {
	t.Helper()

	var found bool
	storer.modeSetMu.Lock()
	defer storer.modeSetMu.Unlock()

	for k, v := range storer.ModeSet {
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

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/postage"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/storage"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology/mock"
)

// no of times to retry to see if we have received response from pushsync
var noOfRetries = 20
var block = common.HexToHash("0x1").Bytes()
var defaultMockValidStamp = func(ch swarm.Chunk, stamp []byte) (swarm.Chunk, error) {
	return ch, nil
}

// Wrap the actual storer to intercept the modeSet that the pusher will call when a valid receipt is received
type Store struct {
	storage.Storer
	internalStorer storage.Storer
	modeSet        map[string]storage.ModeSet
	modeSetMu      *sync.Mutex

	closed              bool
	setBeforeCloseCount int
	setAfterCloseCount  int
}

// Override the Set function to capture the ModeSetSync
func (s *Store) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) error {
	s.modeSetMu.Lock()
	defer s.modeSetMu.Unlock()

	if s.closed {
		s.setAfterCloseCount++
	} else {
		s.setBeforeCloseCount++
	}

	for _, addr := range addrs {
		s.modeSet[addr.String()] = mode
	}
	return nil
}

func (s *Store) Close() error {
	s.modeSetMu.Lock()
	defer s.modeSetMu.Unlock()

	s.closed = true
	return s.internalStorer.Close()
}

// TestSendChunkToPushSync sends a chunk to pushsync to be sent ot its closest peer and get a receipt.
// once the receipt is got this check to see if the localstore is updated to see if the chunk is set
// as ModeSetSync status.
func TestSendChunkToSyncWithTag(t *testing.T) {
	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			BlockHash: block,
		}
		return receipt, nil
	})

	mtags, p, storer := createPusher(t, triggerPeer, pushSyncService, defaultMockValidStamp, mock.WithClosestPeer(closestPeer), mock.WithNeighborhoodDepth(0))
	defer storer.Close()
	defer p.Close()

	ta, err := mtags.Create(1)
	if err != nil {
		t.Fatal(err)
	}

	chunk := testingc.GenerateTestRandomChunk().WithTagID(ta.Uid)

	_, err = storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check if the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(50 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSync, storer)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}

	if ta.Get(tags.StateSynced) != 1 {
		t.Fatalf("tags error")
	}
}

// TestSendChunkToPushSyncWithoutTag is similar to TestSendChunkToPushSync, excep that the tags are not
// present to simulate bzz api withotu splitter condition
func TestSendChunkToPushSyncWithoutTag(t *testing.T) {
	chunk := testingc.GenerateTestRandomChunk()

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			BlockHash: block,
		}
		return receipt, nil
	})

	_, p, storer := createPusher(t, triggerPeer, pushSyncService, defaultMockValidStamp, mock.WithClosestPeer(closestPeer), mock.WithNeighborhoodDepth(0))
	defer storer.Close()
	defer p.Close()

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check if the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(50 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSync, storer)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
}

// TestSendChunkAndReceiveInvalidReceipt sends a chunk to pushsync to be sent ot its closest peer and
// get a invalid receipt (not with the address of the chunk sent). The test makes sure that this error
// is received and the ModeSetSync is not set for the chunk.
func TestSendChunkAndReceiveInvalidReceipt(t *testing.T) {
	chunk := testingc.GenerateTestRandomChunk()

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		return nil, errors.New("invalid receipt")
	})

	_, p, storer := createPusher(t, triggerPeer, pushSyncService, defaultMockValidStamp, mock.WithClosestPeer(closestPeer))
	defer storer.Close()
	defer p.Close()

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check if the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(10 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSync, storer)
		if err != nil {
			continue
		}
	}
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
}

// TestSendChunkAndTimeoutinReceivingReceipt sends a chunk to pushsync to be sent ot its closest peer and
// expects a timeout to get instead of getting a receipt. The test makes sure that timeout error
// is received and the ModeSetSync is not set for the chunk.
func TestSendChunkAndTimeoutinReceivingReceipt(t *testing.T) {
	chunk := testingc.GenerateTestRandomChunk()

	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		time.Sleep(1 * time.Second)
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			BlockHash: block,
		}
		return receipt, nil
	})

	_, p, storer := createPusher(t, triggerPeer, pushSyncService, defaultMockValidStamp, mock.WithClosestPeer(closestPeer), mock.WithNeighborhoodDepth(0))
	defer storer.Close()
	defer p.Close()

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check if the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(10 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSync, storer)
		if err != nil {
			continue
		}
	}
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
}

func TestPusherRetryShallow(t *testing.T) {
	defer func(d time.Duration, retryCount int) {
		*pusher.RetryInterval = d
		*pusher.RetryCount = retryCount
	}(*pusher.RetryInterval, *pusher.RetryCount)
	*pusher.RetryInterval = 500 * time.Millisecond
	*pusher.RetryCount = 3

	var (
		pivotPeer   = swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
		closestPeer = swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")
		key, _      = crypto.GenerateSecp256k1Key()
		signer      = crypto.NewDefaultSigner(key)
		callCount   = int32(0)
	)
	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		atomic.AddInt32(&callCount, 1)
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			BlockHash: block,
		}
		return receipt, nil
	})

	// create the pivot peer pusher with depth 31, this makes
	// sure that virtually any receipt generated by the random
	// key will be considered too shallow
	_, ps, storer := createPusher(t, pivotPeer, pushSyncService, defaultMockValidStamp, mock.WithClosestPeer(closestPeer), mock.WithNeighborhoodDepth(31))
	defer ps.Close()

	// generate a chunk at PO 1 with closestPeer, meaning that we get a
	// receipt which is shallower than the pivot peer's depth, resulting
	// in retries
	chunk := testingc.GenerateTestRandomChunkAt(closestPeer, 1)

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}
	c := 0
	for i := 0; i < 5; i++ {
		c = int(atomic.LoadInt32(&callCount))
		if c == *pusher.RetryCount {
			return
		}
		if c > *pusher.RetryCount {
			t.Fatalf("too many retries. got %d want %d", c, *pusher.RetryCount)
		}
		time.Sleep(1 * time.Second)
	}

	t.Fatalf("timed out waiting for retries. got %d want %d", c, *pusher.RetryCount)
}

// TestChunkWithInvalidStampSkipped tests that chunks with invalid stamps are skipped in pusher
func TestChunkWithInvalidStampSkipped(t *testing.T) {
	// create a trigger  and a closestpeer
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			BlockHash: block,
		}
		return receipt, nil
	})

	validStamp := func(ch swarm.Chunk, stamp []byte) (swarm.Chunk, error) {
		return nil, errors.New("valid stamp error")
	}

	_, p, storer := createPusher(t, triggerPeer, pushSyncService, validStamp, mock.WithClosestPeer(closestPeer), mock.WithNeighborhoodDepth(0))
	defer storer.Close()
	defer p.Close()

	chunk := testingc.GenerateTestRandomChunk()

	_, err := storer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// Check if the chunk is set as synced in the DB.
	for i := 0; i < noOfRetries; i++ {
		// Give some time for chunk to be pushed and receipt to be received
		time.Sleep(50 * time.Millisecond)

		err = checkIfModeSet(chunk.Address(), storage.ModeSetSync, storer)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
}

func createPusher(t *testing.T, addr swarm.Address, pushSyncService pushsync.PushSyncer, validStamp postage.ValidStampFn, mockOpts ...mock.Option) (*tags.Tags, *pusher.Service, *Store) {
	t.Helper()
	logger := logging.New(io.Discard, 0)
	storer, err := localstore.New(context.Background(), "", addr.Bytes(), nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	mockStatestore := statestore.NewStateStore()
	mtags := tags.NewTags(mockStatestore, logger)
	pusherStorer := &Store{
		Storer:         storer,
		internalStorer: storer,
		modeSet:        make(map[string]storage.ModeSet),
		modeSetMu:      &sync.Mutex{},
	}
	peerSuggester := mock.NewTopologyDriver(mockOpts...)

	pusherService := pusher.New(1, pusherStorer, peerSuggester, pushSyncService, validStamp, mtags, logger, nil, 0)
	return mtags, pusherService, pusherStorer
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

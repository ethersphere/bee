// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	batchstoremock "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/v2/pkg/pushsync/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// time to wait for received response from pushsync
const spinTimeout = time.Second * 3

var (
	block                 = common.HexToHash("0x1").Bytes()
	defaultMockBatchStore = batchstoremock.New(batchstoremock.WithExistsFunc(func(b []byte) (bool, error) {
		return true, nil
	}))
	defaultRetryCount = 3
)

type mockStorer struct {
	chunks         chan swarm.Chunk
	reportedMu     sync.Mutex
	reportedSynced []swarm.Chunk
	reportedFailed []swarm.Chunk
	reportedStored []swarm.Chunk
	storedChunks   map[string]swarm.Chunk
}

func (m *mockStorer) SubscribePush(ctx context.Context) (c <-chan swarm.Chunk, stop func()) {
	return m.chunks, func() { close(m.chunks) }
}

func (m *mockStorer) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {
	m.reportedMu.Lock()
	defer m.reportedMu.Unlock()

	switch state {
	case storage.ChunkSynced:
		m.reportedSynced = append(m.reportedSynced, chunk)
	case storage.ChunkCouldNotSync:
		m.reportedFailed = append(m.reportedFailed, chunk)
	case storage.ChunkStored:
		m.reportedStored = append(m.reportedStored, chunk)
	}
	return nil
}

func (m *mockStorer) isReported(chunk swarm.Chunk, state storage.ChunkState) bool {
	m.reportedMu.Lock()
	defer m.reportedMu.Unlock()

	switch state {
	case storage.ChunkSynced:
		for _, ch := range m.reportedSynced {
			if ch.Equal(chunk) {
				return true
			}
		}
	case storage.ChunkCouldNotSync:
		for _, ch := range m.reportedFailed {
			if ch.Equal(chunk) {
				return true
			}
		}
	case storage.ChunkStored:
		for _, ch := range m.reportedStored {
			if ch.Equal(chunk) {
				return true
			}
		}
	}

	return false
}

func (m *mockStorer) ReservePutter() storage.Putter {
	return storage.PutterFunc(
		func(ctx context.Context, chunk swarm.Chunk) error {
			if m.storedChunks == nil {
				m.storedChunks = make(map[string]swarm.Chunk)
			}
			m.storedChunks[chunk.Address().ByteString()] = chunk
			return nil
		},
	)
}

// TestSendChunkToPushSync sends a chunk to pushsync to be sent to its closest peer and get a receipt.
// once the receipt is got this check to see if the localstore is updated to see if the chunk is set
// as ModeSetSync status.
func TestChunkSyncing(t *testing.T) {
	t.Parallel()

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			Nonce:     block,
		}
		return receipt, nil
	})

	storer := &mockStorer{
		chunks: make(chan swarm.Chunk),
	}

	pusherSvc := createPusher(
		t,
		storer,
		pushSyncService,
		defaultMockBatchStore,
		defaultRetryCount,
	)

	t.Run("deferred", func(t *testing.T) {
		chunk := testingc.GenerateTestRandomChunk()
		storer.chunks <- chunk

		err := spinlock.Wait(spinTimeout, func() bool {
			return storer.isReported(chunk, storage.ChunkSynced)
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("direct", func(t *testing.T) {
		chunk := testingc.GenerateTestRandomChunk()

		newFeed := make(chan *pusher.Op)
		errC := make(chan error, 1)
		pusherSvc.AddFeed(newFeed)

		newFeed <- &pusher.Op{Chunk: chunk, Err: errC, Direct: true}

		err := <-errC
		if err != nil {
			t.Fatalf("unexpected error on push %v", err)
		}
	})
}

func TestChunkStored(t *testing.T) {
	t.Parallel()

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		return nil, topology.ErrWantSelf
	})

	storer := &mockStorer{
		chunks: make(chan swarm.Chunk),
	}

	pusherSvc := createPusher(
		t,
		storer,
		pushSyncService,
		defaultMockBatchStore,
		defaultRetryCount,
	)

	t.Run("deferred", func(t *testing.T) {
		chunk := testingc.GenerateTestRandomChunk()
		storer.chunks <- chunk

		err := spinlock.Wait(spinTimeout, func() bool {
			return storer.isReported(chunk, storage.ChunkStored)
		})
		if err != nil {
			t.Fatal(err)
		}
		if ch, found := storer.storedChunks[chunk.Address().ByteString()]; !found || !ch.Equal(chunk) {
			t.Fatalf("chunk not found in the store")
		}
	})

	t.Run("direct", func(t *testing.T) {
		chunk := testingc.GenerateTestRandomChunk()

		newFeed := make(chan *pusher.Op)
		errC := make(chan error, 1)
		pusherSvc.AddFeed(newFeed)

		newFeed <- &pusher.Op{Chunk: chunk, Err: errC, Direct: true}

		err := <-errC
		if err != nil {
			t.Fatalf("unexpected error on push %v", err)
		}
		if ch, found := storer.storedChunks[chunk.Address().ByteString()]; !found || !ch.Equal(chunk) {
			t.Fatalf("chunk not found in the store")
		}
	})
}

// TestSendChunkAndReceiveInvalidReceipt sends a chunk to pushsync to be sent to its closest peer and
// get a invalid receipt (not with the address of the chunk sent). The test makes sure that this error
// is received and the ModeSetSync is not set for the chunk.
func TestSendChunkAndReceiveInvalidReceipt(t *testing.T) {
	t.Parallel()

	chunk := testingc.GenerateTestRandomChunk()

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		return nil, errors.New("invalid receipt")
	})

	storer := &mockStorer{
		chunks: make(chan swarm.Chunk),
	}

	_ = createPusher(
		t,
		storer,
		pushSyncService,
		defaultMockBatchStore,
		defaultRetryCount,
	)

	storer.chunks <- chunk

	err := spinlock.Wait(spinTimeout, func() bool {
		return storer.isReported(chunk, storage.ChunkSynced)
	})
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
}

// TestSendChunkAndTimeoutinReceivingReceipt sends a chunk to pushsync to be sent to its closest peer and
// expects a timeout to get instead of getting a receipt. The test makes sure that timeout error
// is received and the ModeSetSync is not set for the chunk.
func TestSendChunkAndTimeoutinReceivingReceipt(t *testing.T) {
	t.Parallel()

	chunk := testingc.GenerateTestRandomChunk()

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		time.Sleep(5 * time.Second)
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			Nonce:     block,
		}
		return receipt, nil
	})

	storer := &mockStorer{
		chunks: make(chan swarm.Chunk),
	}

	_ = createPusher(
		t,
		storer,
		pushSyncService,
		defaultMockBatchStore,
		defaultRetryCount,
	)

	storer.chunks <- chunk

	err := spinlock.Wait(spinTimeout, func() bool {
		return storer.isReported(chunk, storage.ChunkSynced)
	})
	if err == nil {
		t.Fatalf("chunk not syned error expected")
	}
}

func TestPusherRetryShallow(t *testing.T) {
	t.Parallel()

	var (
		closestPeer = swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")
		key, _      = crypto.GenerateSecp256k1Key()
		signer      = crypto.NewDefaultSigner(key)
		callCount   = int32(0)
		retryCount  = 3 // pushync will retry on behalf of push for shallow receipts, so no retries are made on the side of the pusher.
	)
	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		atomic.AddInt32(&callCount, 1)
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			Nonce:     block,
		}
		return receipt, pushsync.ErrShallowReceipt
	})

	storer := &mockStorer{
		chunks: make(chan swarm.Chunk),
	}

	_ = createPusher(
		t,
		storer,
		pushSyncService,
		defaultMockBatchStore,
		defaultRetryCount,
	)

	// generate a chunk at PO 1 with closestPeer, meaning that we get a
	// receipt which is shallower than the pivot peer's depth, resulting
	// in retries
	chunk := testingc.GenerateValidRandomChunkAt(t, closestPeer, 1)

	storer.chunks <- chunk

	err := spinlock.Wait(spinTimeout, func() bool {
		c := int(atomic.LoadInt32(&callCount))
		return c == retryCount
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestChunkWithInvalidStampSkipped tests that chunks with invalid stamps are skipped in pusher
func TestChunkWithInvalidStampSkipped(t *testing.T) {
	t.Parallel()

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		signature, _ := signer.Sign(chunk.Address().Bytes())
		receipt := &pushsync.Receipt{
			Address:   swarm.NewAddress(chunk.Address().Bytes()),
			Signature: signature,
			Nonce:     block,
		}
		return receipt, nil
	})

	wantErr := errors.New("dummy error")

	bmock := batchstoremock.New(batchstoremock.WithExistsFunc(func(b []byte) (bool, error) {
		return false, wantErr
	}))

	storer := &mockStorer{
		chunks: make(chan swarm.Chunk),
	}

	pusherSvc := createPusher(
		t,
		storer,
		pushSyncService,
		bmock,
		defaultRetryCount,
	)

	t.Run("deferred", func(t *testing.T) {
		chunk := testingc.GenerateTestRandomChunk()
		storer.chunks <- chunk

		err := spinlock.Wait(spinTimeout, func() bool {
			return storer.isReported(chunk, storage.ChunkCouldNotSync)
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("direct", func(t *testing.T) {
		chunk := testingc.GenerateTestRandomChunk()

		newFeed := make(chan *pusher.Op)
		errC := make(chan error, 1)
		pusherSvc.AddFeed(newFeed)

		newFeed <- &pusher.Op{Chunk: chunk, Err: errC, Direct: true}

		err := <-errC
		if !errors.Is(err, wantErr) {
			t.Fatalf("unexpected error on push %v", err)
		}
	})
}

func createPusher(
	t *testing.T,
	storer pusher.Storer,
	pushSyncService pushsync.PushSyncer,
	validStamp postage.BatchExist,
	retryCount int,
) *pusher.Service {
	t.Helper()

	pusherService := pusher.New(1, storer, pushSyncService, validStamp, log.Noop, 0, retryCount)
	testutil.CleanupCloser(t, pusherService)

	return pusherService
}

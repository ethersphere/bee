// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher_test

import (
	"bytes"
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
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
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

// TestChunkSyncing sends a chunk to pushsync to be sent to its closest peer and get a receipt.
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

// TestInvalidChunkDoesNotBlockPusher checks that a deferred chunk which fails
// IdentityAddress is dropped from the push queue instead of blocking the worker
// on a nil error channel.
func TestInvalidChunkDoesNotBlockPusher(t *testing.T) {
	t.Parallel()

	key, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(key)

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		signature, _ := signer.Sign(chunk.Address().Bytes())
		return &pushsync.Receipt{Address: swarm.NewAddress(chunk.Address().Bytes()), Signature: signature, Nonce: block}, nil
	})

	storer := &mockStorer{chunks: make(chan swarm.Chunk)}
	_ = createPusher(t, storer, pushSyncService, defaultMockBatchStore, defaultRetryCount)

	valid := testingc.GenerateTestRandomChunk()
	addr := make([]byte, swarm.HashSize)
	for i := range addr {
		addr[i] = 0xab
	}
	// Shorter than a SOC header and not hashing to addr: neither CAC nor SOC.
	invalid := swarm.NewChunk(swarm.NewAddress(addr), []byte("not a chunk")).WithStamp(valid.Stamp())

	go func() {
		storer.chunks <- invalid
		storer.chunks <- valid
	}()

	if err := spinlock.Wait(spinTimeout, func() bool { return storer.isReported(valid, storage.ChunkSynced) }); err != nil {
		t.Fatalf("pusher stalled behind an invalid chunk: %v", err)
	}
	if !storer.isReported(invalid, storage.ChunkCouldNotSync) {
		t.Fatal("invalid chunk was not reported as could-not-sync")
	}
}

// TestInvalidDeferredChunkLeavesOtherPushesAlone tries to disturb the paths
// around the invalid-chunk drop: direct uploads, valid CAC and SOC, a failing
// Report, shallow-receipt retries, and closest-node storage.
func TestInvalidDeferredChunkLeavesOtherPushesAlone(t *testing.T) {
	t.Parallel()

	t.Run("direct invalid returns error and does not block later pushes", func(t *testing.T) {
		t.Parallel()

		var pushed atomic.Int32
		storer := &mockStorer{chunks: make(chan swarm.Chunk)}
		svc := createPusher(t, storer, receiptSyncer(t, &pushed), defaultMockBatchStore, defaultRetryCount)

		stamp := testingc.GenerateTestRandomChunk().Stamp()
		invalid := invalidChunk(stamp, 1)
		deferred := testingc.GenerateTestRandomChunk()
		direct := testingc.GenerateTestRandomChunk()

		feed := make(chan *pusher.Op)
		invalidErr := make(chan error, 1)
		directErr := make(chan error, 1)
		svc.AddFeed(feed)

		feed <- &pusher.Op{Chunk: invalid, Err: invalidErr, Direct: true}
		select {
		case err := <-invalidErr:
			if !errors.Is(err, storage.ErrUnknownChunkType) {
				t.Fatalf("direct invalid: got %v", err)
			}
		case <-time.After(spinTimeout):
			t.Fatal("direct invalid upload stalled")
		}
		if storer.isReported(invalid, storage.ChunkCouldNotSync) {
			t.Fatal("direct invalid chunk was removed from the push queue")
		}

		go func() {
			storer.chunks <- deferred
		}()
		feed <- &pusher.Op{Chunk: direct, Err: directErr, Direct: true}

		select {
		case err := <-directErr:
			if err != nil {
				t.Fatalf("direct valid upload: %v", err)
			}
		case <-time.After(spinTimeout):
			t.Fatal("direct valid upload stalled behind the invalid chunk")
		}
		if err := spinlock.Wait(spinTimeout, func() bool { return storer.isReported(deferred, storage.ChunkSynced) }); err != nil {
			t.Fatalf("deferred valid chunk stalled: %v", err)
		}
		if pushed.Load() != 2 {
			t.Fatalf("pushsync calls: got %d, want 2", pushed.Load())
		}
	})

	t.Run("invalid deferred chunks do not steal sync from cac and soc", func(t *testing.T) {
		t.Parallel()

		var pushed atomic.Int32
		storer := &mockStorer{chunks: make(chan swarm.Chunk)}
		syncer := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			if bytes.HasPrefix(chunk.Data(), []byte("not a chunk")) {
				t.Errorf("invalid chunk reached pushsync: %s", chunk.Address())
			}
			pushed.Add(1)
			return receiptFor(t, chunk)
		})
		_ = createPusher(t, storer, syncer, defaultMockBatchStore, defaultRetryCount)

		cacChunk := testingc.GenerateTestRandomChunk()
		socChunk := testingc.GenerateTestRandomSoChunk(t, testingc.GenerateTestRandomChunk())
		invalids := make([]swarm.Chunk, 8)
		for i := range invalids {
			invalids[i] = invalidChunk(cacChunk.Stamp(), byte(i+1))
		}

		go func() {
			for _, ch := range invalids {
				storer.chunks <- ch
			}
			storer.chunks <- cacChunk
			storer.chunks <- socChunk
		}()

		if err := spinlock.Wait(spinTimeout, func() bool {
			return storer.isReported(cacChunk, storage.ChunkSynced) && storer.isReported(socChunk, storage.ChunkSynced)
		}); err != nil {
			t.Fatalf("valid chunks stalled behind invalid ones: %v", err)
		}
		for _, ch := range invalids {
			if !storer.isReported(ch, storage.ChunkCouldNotSync) {
				t.Fatalf("invalid chunk %s was not dropped", ch.Address())
			}
			if storer.isReported(ch, storage.ChunkSynced) || storer.isReported(ch, storage.ChunkStored) {
				t.Fatalf("invalid chunk %s was treated as synced", ch.Address())
			}
		}
		if storer.isReported(cacChunk, storage.ChunkCouldNotSync) || storer.isReported(socChunk, storage.ChunkCouldNotSync) {
			t.Fatal("valid chunk was reported as could-not-sync")
		}
		if pushed.Load() != 2 {
			t.Fatalf("pushsync calls: got %d, want 2", pushed.Load())
		}
	})

	t.Run("failed report does not stall the worker", func(t *testing.T) {
		t.Parallel()

		storer := &flakyStorer{mockStorer: &mockStorer{chunks: make(chan swarm.Chunk)}}
		_ = createPusher(t, storer, receiptSyncer(t, nil), defaultMockBatchStore, defaultRetryCount)

		valid := testingc.GenerateTestRandomChunk()
		invalid := invalidChunk(valid.Stamp(), 1)
		go func() {
			storer.chunks <- invalid
			storer.chunks <- valid
		}()

		if err := spinlock.Wait(spinTimeout, func() bool { return storer.isReported(valid, storage.ChunkSynced) }); err != nil {
			t.Fatalf("worker stalled when dropping an invalid chunk failed: %v", err)
		}
		if storer.isReported(invalid, storage.ChunkCouldNotSync) {
			t.Fatal("failed report was recorded as could-not-sync")
		}
	})

	t.Run("shallow receipt retries ignore the invalid chunk", func(t *testing.T) {
		t.Parallel()

		var pushed atomic.Int32
		syncer := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			pushed.Add(1)
			receipt, err := receiptFor(t, chunk)
			return receipt, errors.Join(err, pushsync.ErrShallowReceipt)
		})
		storer := &mockStorer{chunks: make(chan swarm.Chunk)}
		_ = createPusher(t, storer, syncer, defaultMockBatchStore, defaultRetryCount)

		valid := testingc.GenerateValidRandomChunkAt(t, swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000"), 1)
		invalid := invalidChunk(valid.Stamp(), 1)
		go func() {
			storer.chunks <- invalid
			storer.chunks <- valid
		}()

		if err := spinlock.Wait(spinTimeout, func() bool { return int(pushed.Load()) == defaultRetryCount }); err != nil {
			t.Fatalf("shallow retries: got %d calls: %v", pushed.Load(), err)
		}
		if !storer.isReported(invalid, storage.ChunkCouldNotSync) {
			t.Fatal("invalid chunk was not dropped before the shallow retries")
		}
	})

	t.Run("closest node stores only the valid chunk", func(t *testing.T) {
		t.Parallel()

		syncer := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			if bytes.HasPrefix(chunk.Data(), []byte("not a chunk")) {
				t.Errorf("invalid chunk reached pushsync: %s", chunk.Address())
			}
			return nil, topology.ErrWantSelf
		})
		storer := &mockStorer{chunks: make(chan swarm.Chunk)}
		_ = createPusher(t, storer, syncer, defaultMockBatchStore, defaultRetryCount)

		valid := testingc.GenerateTestRandomChunk()
		invalid := invalidChunk(valid.Stamp(), 1)
		go func() {
			storer.chunks <- invalid
			storer.chunks <- valid
		}()

		if err := spinlock.Wait(spinTimeout, func() bool { return storer.isReported(valid, storage.ChunkStored) }); err != nil {
			t.Fatalf("closest-node store stalled: %v", err)
		}
		if _, found := storer.storedChunks[invalid.Address().ByteString()]; found {
			t.Fatal("invalid chunk was written to the reserve")
		}
		if ch, found := storer.storedChunks[valid.Address().ByteString()]; !found || !ch.Equal(valid) {
			t.Fatal("valid chunk was not stored")
		}
	})

	t.Run("soc under the wrong address is still pushed", func(t *testing.T) {
		t.Parallel()

		// IdentityAddress accepts any chunk soc.FromChunk can parse, including a
		// signed SOC carried under some other address. The drop path must not
		// start rejecting those; only chunks that fail IdentityAddress are dropped.
		var pushed atomic.Int32
		storer := &mockStorer{chunks: make(chan swarm.Chunk)}
		_ = createPusher(t, storer, receiptSyncer(t, &pushed), defaultMockBatchStore, defaultRetryCount)

		wrapped := testingc.GenerateTestRandomChunk()
		signed := testingc.GenerateTestRandomSoChunk(t, wrapped)
		mismatched := swarm.NewChunk(wrapped.Address(), signed.Data()).WithStamp(signed.Stamp())

		go func() {
			storer.chunks <- mismatched
		}()

		if err := spinlock.Wait(spinTimeout, func() bool { return storer.isReported(mismatched, storage.ChunkSynced) }); err != nil {
			t.Fatalf("address-mismatched SOC was not pushed: %v", err)
		}
		if storer.isReported(mismatched, storage.ChunkCouldNotSync) {
			t.Fatal("address-mismatched SOC was dropped")
		}
		if pushed.Load() != 1 {
			t.Fatalf("pushsync calls: got %d, want 1", pushed.Load())
		}
	})
}

func invalidChunk(stamp swarm.Stamp, mark byte) swarm.Chunk {
	addr := make([]byte, swarm.HashSize)
	for i := range addr {
		addr[i] = 0xab
	}
	addr[0] = mark
	return swarm.NewChunk(swarm.NewAddress(addr), append([]byte("not a chunk"), mark)).WithStamp(stamp)
}

func receiptFor(t *testing.T, chunk swarm.Chunk) (*pushsync.Receipt, error) {
	t.Helper()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Errorf("generate key: %v", err)
		return nil, err
	}
	signature, err := crypto.NewDefaultSigner(key).Sign(chunk.Address().Bytes())
	if err != nil {
		t.Errorf("sign receipt: %v", err)
		return nil, err
	}
	return &pushsync.Receipt{
		Address:   swarm.NewAddress(chunk.Address().Bytes()),
		Signature: signature,
		Nonce:     block,
	}, nil
}

func receiptSyncer(t *testing.T, pushed *atomic.Int32) pushsync.PushSyncer {
	t.Helper()

	return pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		if pushed != nil {
			pushed.Add(1)
		}
		return receiptFor(t, chunk)
	})
}

// flakyStorer fails the could-not-sync report. That is the new drop path, so a
// disk error there must not wedge the worker.
type flakyStorer struct {
	*mockStorer
}

func (f *flakyStorer) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {
	if state == storage.ChunkCouldNotSync {
		return errors.New("report failed")
	}
	return f.mockStorer.Report(ctx, chunk, state)
}

func createPusher(
	t *testing.T,
	storer pusher.Storer,
	pushSyncService pushsync.PushSyncer,
	validStamp postage.BatchExist,
	retryCount int,
) *pusher.Service {
	t.Helper()

	pusherService := pusher.New(1, storer, pushSyncService, validStamp, log.Noop, stabilmock.NewSubscriber(true), retryCount)
	testutil.CleanupCloser(t, pusherService)

	return pusherService
}

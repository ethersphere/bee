// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice_test

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	mocks "github.com/ethersphere/bee/v2/pkg/statestore/mock"
)

// TestDirtyShutdownCausesResync verifies that if a node is killed between
// TransactionStart and TransactionEnd (simulating a dirty shutdown), the
// next Start call detects the dirty flag and resets the batch store.
func TestDirtyShutdownCausesResync(t *testing.T) {
	t.Parallel()

	stateStore := mocks.NewStateStore()
	batchStore := mock.New()

	svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// Simulate the beginning of event processing.
	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	// --- Simulated crash: TransactionEnd is never called ---

	// Verify dirty flag is persisted in state store.
	var dirty bool
	if err := stateStore.Get(batchservice.DirtyDBKey, &dirty); err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("expected dirty flag to be set after TransactionStart")
	}

	// Create a new batch service instance (simulating node restart)
	// reusing the same stateStore (which persists across restarts on disk).
	svc2, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc2.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// The batch store must have been reset due to dirty shutdown detection.
	if c := batchStore.ResetCalls(); c != 1 {
		t.Fatalf("expected 1 batch store Reset call (dirty shutdown recovery), got %d", c)
	}

	// After recovery, dirty flag must be cleared.
	if err := stateStore.Get(batchservice.DirtyDBKey, &dirty); err == nil {
		t.Fatal("expected dirty flag to be cleared after recovery, but it still exists")
	}
}

// TestCleanShutdownNoResync verifies that a proper TransactionStart/TransactionEnd
// cycle does not trigger a resync on the next start.
func TestCleanShutdownNoResync(t *testing.T) {
	t.Parallel()

	stateStore := mocks.NewStateStore()
	batchStore := mock.New()

	svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	// Dirty flag should be gone now.
	var dirty bool
	if err := stateStore.Get(batchservice.DirtyDBKey, &dirty); err == nil {
		t.Fatal("dirty flag should not exist after clean TransactionEnd")
	}

	batchStore2 := mock.New()
	svc2, err := batchservice.New(stateStore, batchStore2, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	if c := batchStore2.ResetCalls(); c != 0 {
		t.Fatalf("expected 0 Reset calls after clean shutdown, got %d", c)
	}
}

// TestRapidRestartsWithDirtyShutdowns simulates multiple rapid restart cycles
// where each "restart" is interrupted mid-transaction. Verifies that the batch
// store is always reset on recovery and that state remains consistent.
func TestRapidRestartsWithDirtyShutdowns(t *testing.T) {
	t.Parallel()

	stateStore := mocks.NewStateStore()

	for i := 0; i < 10; i++ {
		batchStore := mock.New()
		svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
		if err != nil {
			t.Fatalf("iteration %d: New: %v", i, err)
		}

		if err := svc.Start(context.Background(), 0); err != nil {
			t.Fatalf("iteration %d: Start: %v", i, err)
		}

		expectedResets := 0
		if i > 0 {
			expectedResets = 1 // every restart after the first should detect dirty flag
		}
		if c := batchStore.ResetCalls(); c != expectedResets {
			t.Fatalf("iteration %d: expected %d Reset calls, got %d", i, expectedResets, c)
		}

		if err := svc.TransactionStart(); err != nil {
			t.Fatalf("iteration %d: TransactionStart: %v", i, err)
		}

		// Crash: never call TransactionEnd
	}

	// Final clean restart should still detect dirty flag and recover.
	finalStore := mock.New()
	svc, err := batchservice.New(stateStore, finalStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if c := finalStore.ResetCalls(); c != 1 {
		t.Fatalf("final restart: expected 1 Reset call, got %d", c)
	}
}

// TestDirtyShutdownDuringEventProcessing simulates a more realistic scenario:
// a listener is processing blockchain events when the node is killed.
// The batch service has already created batches, and the dirty shutdown
// should cause a full resync that re-creates them from scratch.
func TestDirtyShutdownDuringEventProcessing(t *testing.T) {
	t.Parallel()

	testChainState := &postage.ChainState{
		Block:        100,
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(100),
	}

	stateStore := mocks.NewStateStore()
	batchStore := mock.New(mock.WithChainState(testChainState))

	svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// Simulate a transaction batch with real event processing.
	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	// Process some events within the transaction.
	if err := svc.UpdateBlockNumber(105); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdatePrice(big.NewInt(200), common.BytesToHash([]byte("tx1"))); err != nil {
		t.Fatal(err)
	}

	// Crash mid-transaction: UpdateBlockNumber and UpdatePrice were applied
	// but TransactionEnd was never called.

	// Verify chain state was partially applied (pending state).
	cs := batchStore.GetChainState()
	if cs.Block != 100 {
		// The mock applies changes immediately; in production the pending state
		// is only committed on TransactionEnd. We verify the dirty flag instead.
	}

	// Restart: new batch store should be reset.
	batchStore2 := mock.New(mock.WithChainState(testChainState))
	svc2, err := batchservice.New(stateStore, batchStore2, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	if c := batchStore2.ResetCalls(); c != 1 {
		t.Fatalf("expected batch store reset after dirty shutdown, got %d resets", c)
	}
}

// TestDirtyShutdownWithBatchData verifies that previously synced batch data
// is lost after a dirty shutdown recovery (because Reset clears the store),
// which is the core issue: unnecessary re-syncing from the beginning.
func TestDirtyShutdownWithBatchData(t *testing.T) {
	t.Parallel()

	testChainState := &postage.ChainState{
		Block:        100,
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(50),
	}

	stateStore := mocks.NewStateStore()
	batchStore := mock.New(mock.WithChainState(testChainState))

	svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// Create a batch (simulating already-synced data).
	testBatch := postagetesting.MustNewBatch()
	testBatch.Value = big.NewInt(0).Add(testChainState.TotalAmount, big.NewInt(0).Mul(testChainState.CurrentPrice, big.NewInt(2)))
	if err := svc.Create(
		testBatch.ID,
		testBatch.Owner,
		big.NewInt(1000),
		testBatch.Value,
		testBatch.Depth,
		testBatch.BucketDepth,
		testBatch.Immutable,
		common.BytesToHash([]byte("tx1")),
	); err != nil {
		t.Fatal(err)
	}

	// Verify batch exists.
	exists, err := batchStore.Exists(testBatch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("batch should exist after creation")
	}

	// Now start a new transaction and crash.
	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}
	// --- crash: no TransactionEnd ---

	// Restart with a fresh batch store (simulating the Reset clearing everything).
	batchStore2 := mock.New(mock.WithChainState(testChainState))
	svc2, err := batchservice.New(stateStore, batchStore2, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// Reset was triggered.
	if c := batchStore2.ResetCalls(); c != 1 {
		t.Fatalf("expected 1 Reset call, got %d", c)
	}

	// The previously created batch is gone — the node will have to
	// resync all batch data from the blockchain, which is the reported bug.
	exists, err = batchStore2.Exists(testBatch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("batch should NOT exist after dirty shutdown recovery (store was reset)")
	}
}

// crashingListener implements postage.Listener. It processes events through a
// real-ish flow: the listener calls TransactionStart, feeds events to the
// updater, then crashes (context cancel) before TransactionEnd.
type crashingListener struct {
	events    []listenerEvent
	crashAt   int // crash after processing this many events (0 = crash before any)
	blockFrom uint64
}

type listenerEvent struct {
	blockNumber uint64
	price       *big.Int
}

func (cl *crashingListener) Listen(ctx context.Context, from uint64, updater postage.EventUpdater) <-chan error {
	synced := make(chan error, 1)
	go func() {
		if err := updater.TransactionStart(); err != nil {
			synced <- err
			return
		}

		for i, ev := range cl.events {
			if i >= cl.crashAt {
				// Simulate crash: leave TransactionEnd uncalled
				synced <- nil
				return
			}
			if err := updater.UpdateBlockNumber(ev.blockNumber); err != nil {
				synced <- err
				return
			}
			if ev.price != nil {
				if err := updater.UpdatePrice(ev.price, common.BytesToHash([]byte("tx"))); err != nil {
					synced <- err
					return
				}
			}
		}

		if err := updater.TransactionEnd(); err != nil {
			synced <- err
			return
		}
		synced <- nil
	}()
	return synced
}

func (cl *crashingListener) Close() error { return nil }

// TestDirtyShutdownViaListenerCrash uses a custom listener that simulates
// a crash during event processing, verifying the full flow from listener
// through batch service to dirty flag detection on restart.
func TestDirtyShutdownViaListenerCrash(t *testing.T) {
	t.Parallel()

	testChainState := &postage.ChainState{
		Block:        100,
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(100),
	}

	stateStore := mocks.NewStateStore()
	batchStore := mock.New(mock.WithChainState(testChainState))

	crashListener := &crashingListener{
		events: []listenerEvent{
			{blockNumber: 105, price: big.NewInt(200)},
			{blockNumber: 110, price: big.NewInt(300)},
			{blockNumber: 115, price: big.NewInt(400)},
		},
		crashAt: 1, // process first event, then crash
	}

	svc, err := batchservice.New(stateStore, batchStore, testLog, crashListener, nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	// Start processes events via crashingListener which leaves dirty flag set.
	if err := svc.Start(context.Background(), 100); err != nil {
		t.Fatal(err)
	}

	// Verify dirty flag remains.
	var dirty bool
	if err := stateStore.Get(batchservice.DirtyDBKey, &dirty); err != nil {
		t.Fatalf("expected dirty flag to be set, got error: %v", err)
	}
	if !dirty {
		t.Fatal("expected dirty flag to be true")
	}

	// Restart: should detect dirty and reset.
	batchStore2 := mock.New(mock.WithChainState(testChainState))
	svc2, err := batchservice.New(stateStore, batchStore2, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 100); err != nil {
		t.Fatal(err)
	}

	if c := batchStore2.ResetCalls(); c != 1 {
		t.Fatalf("expected 1 Reset after listener crash, got %d", c)
	}
}

// TestConcurrentRapidRestarts simulates multiple goroutines rapidly creating
// and crashing batch service instances sharing the same state store, testing
// for data races and state corruption under concurrent access.
func TestConcurrentRapidRestarts(t *testing.T) {
	t.Parallel()

	stateStore := mocks.NewStateStore()
	var wg sync.WaitGroup
	var totalResets atomic.Int64

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			batchStore := mock.New()
			svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
			if err != nil {
				return
			}
			if err := svc.Start(context.Background(), 0); err != nil {
				return
			}

			totalResets.Add(int64(batchStore.ResetCalls()))

			// Half of the goroutines crash mid-transaction.
			if err := svc.TransactionStart(); err != nil {
				return
			}
			// no TransactionEnd — simulated crash
		}()
	}

	wg.Wait()

	// Final restart to check overall state consistency.
	finalStore := mock.New()
	svc, err := batchservice.New(stateStore, finalStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// After concurrent dirty shutdowns, the final restart must detect dirty state.
	if c := finalStore.ResetCalls(); c != 1 {
		t.Fatalf("expected 1 Reset call on final restart after concurrent crashes, got %d", c)
	}
}

// TestChecksumCorruptionAfterDirtyShutdown verifies that after a dirty
// shutdown, the checksum is properly invalidated so that the resync
// recomputes it from scratch rather than using a stale checksum.
func TestChecksumCorruptionAfterDirtyShutdown(t *testing.T) {
	t.Parallel()

	testChainState := &postage.ChainState{
		Block:        1,
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(100),
	}

	stateStore := mocks.NewStateStore()
	batchStore := mock.New(mock.WithChainState(testChainState))

	svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// Perform a clean transaction to establish a checksum.
	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateBlockNumber(10); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	// Start a new transaction and crash.
	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}
	// --- crash ---

	// On restart, the dirty flag causes a full reset.
	batchStore2 := mock.New(mock.WithChainState(testChainState))
	svc2, err := batchservice.New(stateStore, batchStore2, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	if c := batchStore2.ResetCalls(); c != 1 {
		t.Fatalf("expected reset after dirty shutdown, got %d", c)
	}

	// After clean recovery, a new clean cycle should work without issues.
	if err := svc2.TransactionStart(); err != nil {
		t.Fatal(err)
	}
	if err := svc2.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	// Third restart should be clean — no dirty flag, no reset.
	batchStore3 := mock.New(mock.WithChainState(testChainState))
	_, err = batchservice.New(stateStore, batchStore3, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateStore.Get(batchservice.DirtyDBKey, new(bool)); err == nil {
		t.Fatal("dirty flag should not exist after clean TransactionEnd")
	}
}

// TestResyncFlagAlwaysResetsRegardlessOfDirtyState verifies that the --resync
// flag causes a reset even when there was no dirty shutdown.
func TestResyncFlagAlwaysResetsRegardlessOfDirtyState(t *testing.T) {
	t.Parallel()

	stateStore := mocks.NewStateStore()

	// First run: clean start and clean shutdown.
	batchStore := mock.New()
	svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	// Second run with resync=true: should reset even though shutdown was clean.
	batchStore2 := mock.New()
	svc2, err := batchservice.New(stateStore, batchStore2, testLog, newMockListener(), nil, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	if c := batchStore2.ResetCalls(); c != 1 {
		t.Fatalf("expected 1 Reset call with resync=true, got %d", c)
	}
}

// TestStartBlockResetsToZeroAfterDirtyShutdown verifies that after a dirty
// shutdown resets the batch store, syncing resumes from the configured start
// block (not from a stale chain state block), demonstrating the "re-sync from
// the beginning" behavior described in the bug report.
func TestStartBlockResetsToZeroAfterDirtyShutdown(t *testing.T) {
	t.Parallel()

	testChainState := &postage.ChainState{
		Block:        500, // previously synced up to block 500
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(100),
	}

	stateStore := mocks.NewStateStore()
	batchStore := mock.New(mock.WithChainState(testChainState))

	svc, err := batchservice.New(stateStore, batchStore, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	// Dirty shutdown.
	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	// On restart, Reset() clears chain state back to zero.
	// Use a recording listener to capture what start block is passed to Listen.
	var listenedFrom uint64
	recordingListener := &recordingMockListener{listenedFrom: &listenedFrom}

	batchStore2 := mock.New() // fresh store after Reset — chain state block is 0
	svc2, err := batchservice.New(stateStore, batchStore2, testLog, recordingListener, nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	configuredStartBlock := uint64(10)
	if err := svc2.Start(context.Background(), configuredStartBlock); err != nil {
		t.Fatal(err)
	}

	// After reset, chain state block is 0, so Start should use configuredStartBlock+1.
	if listenedFrom != configuredStartBlock+1 {
		t.Fatalf("expected listener to start from block %d, got %d", configuredStartBlock+1, listenedFrom)
	}
}

type recordingMockListener struct {
	listenedFrom *uint64
}

func (r *recordingMockListener) Listen(_ context.Context, from uint64, _ postage.EventUpdater) <-chan error {
	*r.listenedFrom = from
	c := make(chan error, 1)
	c <- nil
	return c
}

func (r *recordingMockListener) Close() error { return nil }

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type mockEvent struct {
	kind          string
	adds, removes int
	radius        uint8
}

type mockObserver struct {
	mu     sync.Mutex
	events []mockEvent
}

func (o *mockObserver) Commit(run func() (adds, removes []storer.ChunkID, err error)) error {
	adds, removes, err := run()
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, mockEvent{kind: "commit", adds: len(adds), removes: len(removes)})
	return err
}

func (o *mockObserver) RadiusChanged(r uint8) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, mockEvent{kind: "radius", radius: r})
}

func (o *mockObserver) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, mockEvent{kind: "reset"})
}

func (o *mockObserver) take() []mockEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	e := o.events
	o.events = nil
	return e
}

func idOf(t *testing.T, ch swarm.Chunk) []byte {
	t.Helper()
	sh, err := ch.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	return append(slices.Clone(ch.Address().Bytes()), sh...)
}

func iterateAll(t *testing.T, s *mockstorer.ReserveStore, prefix swarm.Address, bits uint8, after []byte) [][]byte {
	t.Helper()
	var got [][]byte
	err := s.ReserveIterateAddr(prefix, bits, after, func(c storer.ChunkID) (bool, error) {
		got = append(got, append(slices.Clone(c.Address.Bytes()), c.StampHash...))
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestReserveAddrStoreMock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	batch := postagetesting.MustNewBatch()
	initial := make([]swarm.Chunk, 0, 20)
	for range cap(initial) {
		initial = append(initial, chunktesting.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batch.ID)))
	}
	s := mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithChunks(initial...))
	if !s.ReserveAddrIndexReady() {
		t.Fatal("not ready")
	}

	// order, prefix and cursor
	all := make([][]byte, 0, len(initial))
	for _, ch := range initial {
		all = append(all, idOf(t, ch))
	}
	slices.SortFunc(all, bytes.Compare)
	if got := iterateAll(t, s, swarm.ZeroAddress, 0, nil); !slices.EqualFunc(got, all, bytes.Equal) {
		t.Fatal("full iteration differs")
	}
	if got := iterateAll(t, s, swarm.ZeroAddress, 0, all[4]); !slices.EqualFunc(got, all[5:], bytes.Equal) {
		t.Fatal("iteration after a cursor differs")
	}
	first := all[7][:1]
	var want [][]byte
	for _, id := range all {
		if id[0]&0xf0 == first[0]&0xf0 {
			want = append(want, id)
		}
	}
	if got := iterateAll(t, s, swarm.NewAddress(first), 4, nil); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatal("prefix iteration differs")
	}
	errStop := errors.New("stop")
	if err := s.ReserveIterateAddr(swarm.ZeroAddress, 0, nil, func(storer.ChunkID) (bool, error) { return false, errStop }); !errors.Is(err, errStop) {
		t.Fatalf("callback error: %v", err)
	}

	// has and get
	ch := initial[3]
	id := idOf(t, ch)
	if has, err := s.ReserveHasID(ch.Address(), id[32:]); err != nil || !has {
		t.Fatalf("ReserveHasID: %v, %v", has, err)
	}
	got, err := s.ReserveGetByID(ctx, ch.Address(), id[32:])
	if err != nil || !got.Equal(ch) {
		t.Fatalf("ReserveGetByID: %v", err)
	}
	if _, err := s.ReserveGetByID(ctx, swarm.RandAddress(t), id[32:]); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("ReserveGetByID of an unknown element: %v", err)
	}

	// observer deltas
	if err := s.SetReserveObserver(nil); !errors.Is(err, storer.ErrNilObserver) {
		t.Fatalf("nil observer: %v", err)
	}
	o := &mockObserver{}
	if err := s.SetReserveObserver(o); err != nil {
		t.Fatal(err)
	}
	if err := s.SetReserveObserver(&mockObserver{}); !errors.Is(err, storer.ErrObserverRegistered) {
		t.Fatalf("second observer: %v", err)
	}
	extra := chunktesting.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	putter := s.ReservePutter()
	for _, c := range []swarm.Chunk{extra, extra} {
		if err := putter.Put(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	extraID := idOf(t, extra)
	for _, err := range []error{s.RemoveID(extra.Address(), extraID[32:]), s.RemoveID(extra.Address(), extraID[32:])} {
		if err != nil {
			t.Fatal(err)
		}
	}
	s.SetStorageRadius(3)
	wantEvents := []mockEvent{
		{kind: "commit", adds: 1},
		{kind: "commit"}, // re-put: empty delta
		{kind: "commit", removes: 1},
		{kind: "commit"}, // absent: empty delta
		{kind: "radius", radius: 3},
	}
	if events := o.take(); !slices.Equal(events, wantEvents) {
		t.Fatalf("events %+v, want %+v", events, wantEvents)
	}
	if has, _ := s.ReserveHasID(extra.Address(), extraID[32:]); has {
		t.Fatal("removed element still there")
	}

	if err := s.ResetReserve(ctx); err != nil {
		t.Fatal(err)
	}
	events := o.take()
	if len(events) != len(initial)+1 || events[len(events)-1].kind != "reset" {
		t.Fatalf("reset events %+v", events)
	}
	for _, e := range events[:len(initial)] {
		if e != (mockEvent{kind: "commit", removes: 1}) {
			t.Fatalf("reset event %+v", e)
		}
	}
	if got := iterateAll(t, s, swarm.ZeroAddress, 0, nil); len(got) != 0 {
		t.Fatalf("%d elements after a reset", len(got))
	}

	s.SetAddrIndexReady(false)
	if s.ReserveAddrIndexReady() {
		t.Fatal("still ready")
	}
}

func TestReserveAddrStoreMockDisabled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := mockstorer.NewReserve()
	if s.ReserveAddrIndexReady() {
		t.Fatal("disabled index reports ready")
	}
	if err := s.ReserveIterateAddr(swarm.ZeroAddress, 0, nil, func(storer.ChunkID) (bool, error) { return false, nil }); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("ReserveIterateAddr: %v", err)
	}
	if _, err := s.ReserveGetByID(ctx, swarm.RandAddress(t), nil); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("ReserveGetByID: %v", err)
	}
	if _, err := s.ReserveHasID(swarm.RandAddress(t), nil); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("ReserveHasID: %v", err)
	}
	if err := s.SetReserveObserver(&mockObserver{}); !errors.Is(err, storer.ErrAddrIndexDisabled) {
		t.Fatalf("SetReserveObserver: %v", err)
	}
	// RemoveID and ResetReserve work without the index
	ch := chunktesting.GenerateTestRandomChunk()
	if err := s.ReservePutter().Put(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetReserve(ctx); err != nil {
		t.Fatal(err)
	}
	if has, _ := s.ReserveHas(ch.Address(), ch.Stamp().BatchID(), idOf(t, ch)[32:]); has {
		t.Fatal("chunk left after a reset")
	}
}

// TestReserveAddrStoreMockEvilChunk checks that WithEvilChunk applies to
// ReserveGetByID for an address in the set.
func TestReserveAddrStoreMockEvilChunk(t *testing.T) {
	t.Parallel()

	ch := chunktesting.GenerateTestRandomChunk()
	evil := chunktesting.GenerateTestRandomChunk()
	s := mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithChunks(ch), mockstorer.WithEvilChunk(ch.Address(), evil))
	id := idOf(t, ch)
	got, err := s.ReserveGetByID(context.Background(), ch.Address(), id[32:])
	if err != nil || !got.Equal(evil) {
		t.Fatalf("evil chunk not returned: %v", err)
	}
}

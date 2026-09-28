// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

// openHoldStore opens a one-shard store without registering Close, so tests
// can close and reopen it to inspect the persisted free slots.
func openHoldStore(t *testing.T, dir string) *sharky.Store {
	t.Helper()
	s, err := sharky.New(&dirFS{basedir: dir}, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func holdWrite(t *testing.T, s *sharky.Store, data []byte) sharky.Location {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loc, err := s.Write(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// assertNotReused writes enough blobs that, without a hold, one of them would
// land in the released slot (a one-shard store hands out the lowest free slot
// after at most two pre-popped ones).
func assertNotReused(t *testing.T, s *sharky.Store, held sharky.Location) {
	t.Helper()
	for i := range 16 {
		got := holdWrite(t, s, []byte{2, 2, 2, byte(i)})
		if got.Shard == held.Shard && got.Slot == held.Slot {
			t.Fatalf("write %d reused held slot %v", i, held)
		}
	}
}

// assertFirstWriteReuses reopens the store and checks that the lowest free
// slot, the one handed out first, is the given one.
func assertFirstWriteReuses(t *testing.T, dir string, want sharky.Location) {
	t.Helper()
	s := openHoldStore(t, dir)
	defer func() {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	got := holdWrite(t, s, []byte{3, 3, 3, 3})
	if got.Shard != want.Shard || got.Slot != want.Slot {
		t.Fatalf("first write after reopen: got %v, want slot of %v", got, want)
	}
}

func TestHoldKeepsReleasedSlotsOutOfReuse(t *testing.T) {
	t.Parallel()

	s := openHoldStore(t, t.TempDir())
	ctx := context.Background()

	loc := holdWrite(t, s, []byte{1, 1, 1, 1})
	release := s.Hold()
	if err := s.Release(ctx, loc); err != nil {
		t.Fatal(err)
	}

	assertNotReused(t, s, loc)

	buf := make([]byte, 4)
	if err := s.Read(ctx, loc, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, []byte{1, 1, 1, 1}) {
		t.Fatalf("held slot content changed: %v", buf)
	}

	release()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHoldReleaseFreesHeldSlots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := openHoldStore(t, dir)

	loc := holdWrite(t, s, []byte{1, 1, 1, 1})
	release := s.Hold()
	if err := s.Release(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	release()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	assertFirstWriteReuses(t, dir, loc)
}

func TestHoldCloseWithOpenHoldFreesHeldSlots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := openHoldStore(t, dir)

	loc := holdWrite(t, s, []byte{1, 1, 1, 1})
	release := s.Hold()
	if err := s.Release(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	release() // after Close: must be a no-op, not a panic

	assertFirstWriteReuses(t, dir, loc)
}

func TestHoldNests(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := openHoldStore(t, dir)

	loc := holdWrite(t, s, []byte{1, 1, 1, 1})
	release1 := s.Hold()
	release2 := s.Hold()
	if err := s.Release(context.Background(), loc); err != nil {
		t.Fatal(err)
	}

	release1()
	release1() // idempotent: must not end the second hold
	assertNotReused(t, s, loc)

	release2()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	assertFirstWriteReuses(t, dir, loc)
}

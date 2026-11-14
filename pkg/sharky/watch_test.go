// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky_test

import (
	"context"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

func openWatchStore(t *testing.T) *sharky.Store {
	t.Helper()
	s, err := sharky.New(&dirFS{basedir: t.TempDir()}, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return s
}

func watchWrite(t *testing.T, s *sharky.Store) sharky.Location {
	t.Helper()
	loc, err := s.Write(context.Background(), []byte{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestWatchSeesReleaseBeforeItReturns(t *testing.T) {
	t.Parallel()

	s := openWatchStore(t)
	loc := watchWrite(t, s)

	var got []sharky.Location
	stop := s.Watch(func(l sharky.Location) { got = append(got, l) })
	defer stop()

	if err := s.Release(context.Background(), loc); err != nil {
		t.Fatal(err)
	}
	if want := []sharky.Location{loc}; !slices.Equal(got, want) {
		t.Fatalf("watched releases: got %v, want %v", got, want)
	}
}

func TestWatchStop(t *testing.T) {
	t.Parallel()

	s := openWatchStore(t)
	locs := []sharky.Location{watchWrite(t, s), watchWrite(t, s)}

	var stopped, kept []sharky.Location
	stop := s.Watch(func(l sharky.Location) { stopped = append(stopped, l) })
	defer s.Watch(func(l sharky.Location) { kept = append(kept, l) })()

	stop()
	stop() // idempotent: must not remove the other watcher

	for _, loc := range locs {
		if err := s.Release(context.Background(), loc); err != nil {
			t.Fatal(err)
		}
	}
	if len(stopped) != 0 {
		t.Fatalf("stopped watcher saw %v", stopped)
	}
	if !slices.Equal(kept, locs) {
		t.Fatalf("remaining watcher: got %v, want %v", kept, locs)
	}
}

// TestWatchRunsBeforeSlotIsFreed shows that a Write cannot receive a released
// slot until the watcher has returned.
func TestWatchRunsBeforeSlotIsFreed(t *testing.T) {
	t.Parallel()

	s := openWatchStore(t)
	loc := watchWrite(t, s)

	entered := make(chan struct{})
	gate := make(chan struct{})
	stop := s.Watch(func(sharky.Location) {
		close(entered)
		<-gate
	})
	defer stop()

	released := make(chan error, 1)
	go func() { released <- s.Release(context.Background(), loc) }()
	<-entered

	// The shard holds one popped slot and the slot tracker offers the next one,
	// so a freed slot reaches the third write after it. Six writes leave margin.
	for i := range 6 {
		if got := watchWrite(t, s); got.Slot == loc.Slot {
			t.Fatalf("write %d received slot %d before the watcher returned", i, loc.Slot)
		}
	}

	close(gate)
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	for range 6 {
		if got := watchWrite(t, s); got.Slot == loc.Slot {
			return
		}
	}
	t.Fatalf("slot %d was not reused after Release returned", loc.Slot)
}

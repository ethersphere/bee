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

// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sequence_test

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/feeds/sequence"
	feedstesting "github.com/ethersphere/bee/v2/pkg/feeds/testing"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestFinder(t *testing.T) {
	t.Parallel()

	testf := func(t *testing.T, finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {
		t.Helper()

		t.Run("basic", func(t *testing.T) {
			feedstesting.TestFinderBasic(t, finderf, updaterf)
		})

		t.Run("fixed", func(t *testing.T) {
			i := 0
			nextf := func() (bool, int64) {
				i++
				return i == 40, int64(i)
			}
			feedstesting.TestFinderFixIntervals(t, nextf, finderf, updaterf)
		})

		t.Run("random", func(t *testing.T) {
			feedstesting.TestFinderRandomIntervals(t, finderf, updaterf)
		})
	}

	t.Run("sync", func(t *testing.T) {
		t.Parallel()
		testf(t, sequence.NewFinder, sequence.NewUpdater)
	})

	t.Run("async", func(t *testing.T) {
		t.Parallel()
		testf(t, sequence.NewAsyncFinder, sequence.NewUpdater)
	})
}

func TestAsyncFinderAfter(t *testing.T) {
	t.Parallel()

	newFeed := func(t *testing.T, indexes ...uint64) (storage.ChunkStore, *feeds.Feed) {
		t.Helper()

		store := inmemchunkstore.New()
		key, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		putter, err := feeds.NewPutter(store, crypto.NewDefaultSigner(key), []byte("topic"))
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range indexes {
			if err := putter.Put(context.Background(), sequence.NewIndex(i), payloadOf(i)); err != nil {
				t.Fatal(err)
			}
		}
		return store, putter.Feed
	}

	check := func(t *testing.T, store storage.Getter, feed *feeds.Feed, after, wantIndex uint64) {
		t.Helper()

		ch, cur, next, err := sequence.NewAsyncFinder(store, feed).At(context.Background(), 0, after)
		if err != nil {
			t.Fatal(err)
		}
		if ch == nil {
			t.Fatal("expected to find update, got none")
		}
		if got, want := cur.String(), strconv.FormatUint(wantIndex, 10); got != want {
			t.Fatalf("current index: got %s, want %s", got, want)
		}
		if wantIndex == math.MaxUint64 {
			if next != nil {
				t.Fatalf("next index: got %s, want none after the last possible index", next)
			}
		} else if got, want := next.String(), strconv.FormatUint(wantIndex+1, 10); got != want {
			t.Fatalf("next index: got %s, want %s", got, want)
		}
		wc, err := feeds.FromChunk(ch)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := wc.Data()[swarm.SpanSize:], payloadOf(wantIndex); !bytes.Equal(got, want) {
			t.Fatalf("payload: got %q, want %q", got, want)
		}
	}

	const latest = 299
	indexes := make([]uint64, latest+1)
	for i := range indexes {
		indexes[i] = uint64(i)
	}
	store, feed := newFeed(t, indexes...)

	for _, after := range []uint64{0, 1, 5, 44, 256, latest} {
		t.Run(fmt.Sprintf("start at existing update %d", after), func(t *testing.T) {
			t.Parallel()
			check(t, store, feed, after, latest)
		})
	}

	t.Run("start beyond the latest update", func(t *testing.T) {
		t.Parallel()
		check(t, store, feed, latest+100, latest)
	})

	t.Run("no updates", func(t *testing.T) {
		t.Parallel()

		store, feed := newFeed(t)
		for _, after := range []uint64{0, 7} {
			ch, cur, next, err := sequence.NewAsyncFinder(store, feed).At(context.Background(), 0, after)
			if err != nil {
				t.Fatal(err)
			}
			if ch != nil || cur != nil {
				t.Fatalf("after %d: expected no update, got index %v", after, cur)
			}
			if got := next.String(); got != "0" {
				t.Fatalf("after %d: next index: got %s, want 0", after, got)
			}
		}
	})

	t.Run("start near the end of the index space", func(t *testing.T) {
		t.Parallel()

		// updates at low indexes must not be reached by an overflowing lookup from the end
		store, feed := newFeed(t, 0, 1, 2, 3, math.MaxUint64-1)
		check(t, store, feed, math.MaxUint64-1, math.MaxUint64-1)
	})

	t.Run("update at the last possible index", func(t *testing.T) {
		t.Parallel()

		store, feed := newFeed(t, math.MaxUint64-1, math.MaxUint64)
		check(t, store, feed, math.MaxUint64-1, math.MaxUint64)
	})
}

func payloadOf(i uint64) []byte {
	return []byte("update " + strconv.FormatUint(i, 10))
}

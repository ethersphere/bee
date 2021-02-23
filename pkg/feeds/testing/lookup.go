// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package testing provides tests for update  and resolution of time-based feeds
package testing

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Timeout struct {
	storage.Storer
}

var searchTimeout = 30 * time.Millisecond

// Get overrides the mock storer and introduces latency
func (t *Timeout) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (swarm.Chunk, error) {
	ch, err := t.Storer.Get(ctx, mode, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			time.Sleep(searchTimeout)
		}
		return ch, err
	}
	time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
	return ch, nil
}

func TestFinderBasic(t *testing.T, finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {
	storer := &Timeout{mock.NewStorer()}
	topicStr := "testtopic"
	topic, err := crypto.LegacyKeccak256([]byte(topicStr))
	if err != nil {
		t.Fatal(err)
	}

	pk, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(pk)

	updater, err := updaterf(storer, signer, topic)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	finder := finderf(storer, updater.Feed())
	t.Run("no update", func(t *testing.T) {
		ch, err := feeds.Latest(ctx, finder, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ch != nil {
			t.Fatalf("expected no update, got addr %v", ch.Address())
		}
	})
	t.Run("first update", func(t *testing.T) {
		payload := []byte("payload")
		at := time.Now().Unix()
		err = updater.Update(ctx, at, payload)
		if err != nil {
			t.Fatal(err)
		}
		ch, err := feeds.Latest(ctx, finder, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ch == nil {
			t.Fatalf("expected to find update, got none")
		}
		exp := payload
		ts, payload, err := feeds.FromChunk(ch)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(payload, exp) {
			t.Fatalf("result mismatch. want %8x... got %8x...", exp, payload)
		}
		if ts != uint64(at) {
			t.Fatalf("timestamp mismatch: expected %v, got %v", at, ts)
		}
	})
}

func TestFinderFixIntervals(t *testing.T, nextf func() (bool, int64), finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {
	var stop bool
	for j := 10; !stop; j += 10 {
		t.Run(fmt.Sprintf("custom intervals up to %d", j), func(t *testing.T) {
			var i int64
			var n int
			f := func() (bool, int64) {
				n++
				stop, i = nextf()
				return n == j || stop, i
			}
			TestFinderIntervals(t, f, finderf, updaterf)
		})
	}
}

func TestFinderIntervals(t *testing.T, nextf func() (bool, int64), finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {

	storer := &Timeout{mock.NewStorer()}
	topicStr := "testtopic"
	topic, err := crypto.LegacyKeccak256([]byte(topicStr))
	if err != nil {
		t.Fatal(err)
	}
	pk, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(pk)

	updater, err := updaterf(storer, signer, topic)
	if err != nil {
		t.Fatal(err)
	}
	finder := finderf(storer, updater.Feed())

	ctx := context.Background()
	var ats []int64
	for stop, at := nextf(); !stop; stop, at = nextf() {
		ats = append(ats, at)
		payload := make([]byte, 8)
		binary.BigEndian.PutUint64(payload, uint64(at))
		err = updater.Update(ctx, at, payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	for j := 0; j < len(ats)-1; j++ {
		at := ats[j]
		diff := ats[j+1] - at
		for now := at; now < ats[j+1]; now += int64(rand.Intn(int(diff)) + 1) {
			after := int64(0)
			ch, current, next, err := finder.At(ctx, now, after)
			if err != nil {
				t.Fatal(err)
			}
			if ch == nil {
				t.Fatalf("expected to find update, got none")
			}
			ts, payload, err := feeds.FromChunk(ch)
			if err != nil {
				t.Fatal(err)
			}
			content := binary.BigEndian.Uint64(payload)
			if content != uint64(at) {
				t.Fatalf("payload mismatch: expected %v, got %v", at, content)
			}

			if ts != uint64(at) {
				t.Fatalf("timestamp mismatch: expected %v, got %v", at, ts)
			}

			if current != nil {
				expectedId := ch.Data()[:32]
				id, err := feeds.Id(topic, current)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(id, expectedId) {
					t.Fatalf("current mismatch: expected %x, got %x", expectedId, id)
				}
			}
			if next != nil {
				expectedNext := current.Next(at, uint64(now))
				expectedIdx, err := expectedNext.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				idx, err := next.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(idx, expectedIdx) {
					t.Fatalf("next mismatch: expected %x, got %x", expectedIdx, idx)
				}
			}
		}
	}
}

func TestFinderRandomIntervals(t *testing.T, finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {
	for j := 0; j < 3; j++ {
		t.Run(fmt.Sprintf("random intervals %d", j), func(t *testing.T) {
			var i int64
			var n int
			nextf := func() (bool, int64) {
				i += int64(rand.Intn(1<<10) + 1)
				n++
				return n == 40, i
			}
			TestFinderIntervals(t, nextf, finderf, updaterf)
		})
	}
}

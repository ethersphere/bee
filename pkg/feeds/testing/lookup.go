// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package testing provides tests for update  and resolution of time-based feeds
package testing

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
)

func TestFinderBasic(t *testing.T, finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {
	storer := mock.NewStorer()
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

	storer := mock.NewStorer()
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
	payload := make([]byte, 8)
	var timesNow []int64
	for stop, timeNow := nextf(); !stop; stop, timeNow = nextf() {
		timesNow = append(timesNow, timeNow)
		binary.BigEndian.PutUint64(payload, uint64(timeNow))
		err = updater.Update(ctx, timeNow, payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	finder := finderf(storer, updater.Feed())
	for i, timeNow := range timesNow {
		d := int64(3)
		if i < len(timesNow)-1 {
			d = timesNow[i+1] - timeNow
		}
		step := d / 3
		if step == 0 {
			step = 1
		}
		for now := timeNow; now < timeNow+d; now += step {
			ch, current, next, err := finder.At(ctx, now, 0)
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
			if content != uint64(timeNow) {
				t.Fatalf("payload mismatch: expected %v, got %v", timeNow, content)
			}

			if ts != uint64(timeNow) {
				t.Fatalf("timestamp mismatch: expected %v, got %v", timeNow, ts)
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
				expectedNext := current.Next(timeNow, uint64(timeNow))
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
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("random intervals %d", i), func(t *testing.T) {
			storer := mock.NewStorer()
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

			payload := []byte("payload")
			var at int64
			ats := make([]int64, 100)
			for j := 0; j < 50; j++ {
				ats[j] = at
				at += int64(rand.Intn(1<<10) + 1)
				err = updater.Update(ctx, ats[j], payload)
				if err != nil {
					t.Fatal(err)
				}
			}
			finder := finderf(storer, updater.Feed())
			for j := 1; j < 49; j++ {
				diff := ats[j+1] - ats[j]
				for at := ats[j]; at < ats[j+1]; at += int64(rand.Intn(int(diff)) + 1) {
					for after := int64(0); after < at; after += int64(rand.Intn(int(at))) {
						ch, _, _, err := finder.At(ctx, at, after)
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
							t.Fatalf("payload mismatch: expected %x, got %x", exp, payload)
						}
						if ts != uint64(ats[j]) {
							t.Fatalf("timestamp mismatch: expected %v, got %v", ats[j], ts)
						}
					}
				}
			}
		})
	}
}

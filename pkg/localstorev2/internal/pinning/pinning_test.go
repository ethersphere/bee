// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/storage"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

type pinningCollection struct {
	root         swarm.Chunk
	uniqueChunks []swarm.Chunk
	dupChunks    []swarm.Chunk
}

func TestPinStore(t *testing.T) {

	var tests []pinningCollection

	for _, tc := range []struct {
		dupChunks    int
		uniqueChunks int
	}{
		{
			dupChunks:    5,
			uniqueChunks: 10,
		},
		{
			dupChunks:    10,
			uniqueChunks: 20,
		},
		{
			dupChunks:    15,
			uniqueChunks: 30,
		},
	} {
		var c pinningCollection
		c.root = chunktest.GenerateTestRandomChunk()
		c.uniqueChunks = chunktest.GenerateTestRandomChunks(tc.uniqueChunks)
		dupChunk := chunktest.GenerateTestRandomChunk()
		for i := 0; i < tc.dupChunks; i++ {
			c.dupChunks = append(c.dupChunks, dupChunk)
		}
		tests = append(tests, c)
	}

	st := inmem.New()
	chSt := inmemchunkstore.New()

	t.Run("create new collections", func(t *testing.T) {
		for tCount, tc := range tests {
			t.Run(fmt.Sprintf("create collection %d", tCount), func(t *testing.T) {
				putter, err := pinstore.NewCollection(st, chSt, tc.root.Address())
				if err != nil {
					t.Fatal(err)
				}
				for _, ch := range append(tc.uniqueChunks, tc.root) {
					exists, err := putter.Put(context.TODO(), ch)
					if err != nil {
						t.Fatal(err)
					}
					if exists {
						t.Fatal("chunk should not exist")
					}
				}
				for idx, ch := range tc.dupChunks {
					exists, err := putter.Put(context.TODO(), ch)
					if err != nil {
						t.Fatal(err)
					}
					if !exists && idx != 0 {
						t.Fatal("chunk should exist")
					}
				}
				err = putter.Close()
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	t.Run("verify all collection data", func(t *testing.T) {
		for tCount, tc := range tests {
			t.Run(fmt.Sprintf("verify collection %d", tCount), func(t *testing.T) {
				allChunks := append(tc.uniqueChunks, tc.root)
				allChunks = append(allChunks, tc.dupChunks...)
				for _, ch := range allChunks {
					exists, err := chSt.Has(context.TODO(), ch.Address())
					if err != nil {
						t.Fatal(err)
					}
					if !exists {
						t.Fatal("chunk should exist")
					}
					rch, err := chSt.Get(context.TODO(), ch.Address())
					if err != nil {
						t.Fatal(err)
					}
					if !ch.Equal(rch) {
						t.Fatal("read chunk not equal")
					}
				}
			})
		}
	})

	t.Run("verify root pins", func(t *testing.T) {
		pins, err := pinstore.Pins(st)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 3 {
			t.Fatalf("incorrect no of root pins, expected 3 found %d", len(pins))
		}
		for _, tc := range tests {
			found := false
			for _, f := range pins {
				if f.Equal(tc.root.Address()) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("pin %s not found", tc.root.Address())
			}
		}
	})

	t.Run("has pin", func(t *testing.T) {
		for _, tc := range tests {
			found, err := pinstore.HasPin(st, tc.root.Address())
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf("expected the pin %s to be found", tc.root.Address())
			}
		}
	})

	t.Run("verify internal state", func(t *testing.T) {
		for _, tc := range tests {
			count := 0
			err := pinstore.IterateCollection(st, tc.root.Address(), func(addr swarm.Address) (bool, error) {
				count++
				return false, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if count != len(tc.uniqueChunks)+2 {
				t.Fatalf("incorrect no of chunks in collection, expected %d found %d", len(tc.uniqueChunks)+2, count)
			}
			stat, err := pinstore.GetStat(st, tc.root.Address())
			if err != nil {
				t.Fatal(err)
			}
			if stat.Total != uint64(len(tc.uniqueChunks)+len(tc.dupChunks)+1) {
				t.Fatalf("incorrect no of chunks, expected %d found %d", len(tc.uniqueChunks)+len(tc.dupChunks)+1, stat.Total)
			}
			if stat.DupInCollection != uint64(len(tc.dupChunks)-1) {
				t.Fatalf("incorrect no of duplicate chunks, expected %d found %d", len(tc.dupChunks)-1, stat.DupInCollection)
			}
		}
	})

	t.Run("delete collection", func(t *testing.T) {
		err := pinstore.DeletePin(st, chSt, tests[0].root.Address())
		if err != nil {
			t.Fatal(err)
		}

		found, err := pinstore.HasPin(st, tests[0].root.Address())
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Fatal("expected pin to not be found")
		}

		pins, err := pinstore.Pins(st)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 2 {
			t.Fatalf("incorrect no of root pins, expected 2 found %d", len(pins))
		}

		allChunks := append(tests[0].uniqueChunks, tests[0].root)
		allChunks = append(allChunks, tests[0].dupChunks...)
		for _, ch := range allChunks {
			exists, err := chSt.Has(context.TODO(), ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatal("chunk should not exist")
			}
			rch, err := chSt.Get(context.TODO(), ch.Address())
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatal(err)
			}
		}
	})
}

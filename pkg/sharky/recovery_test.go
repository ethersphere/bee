// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
)

func TestMissingShard(t *testing.T) {
	_, err := sharky.NewRecovery(t.TempDir(), 1, 8)
	if !errors.Is(err, sharky.ErrShardNotFound) {
		t.Fatalf("want %v, got %v", sharky.ErrShardNotFound, err)
	}
}

func TestRecovery(t *testing.T) {
	datasize := 4
	shards := 8
	shardSize := uint32(16)
	limitInChunks := shards * int(shardSize)

	dir := t.TempDir()
	ctx := context.Background()
	size := limitInChunks / 2
	data := make([]byte, 4)
	locs := make([]sharky.Location, size)
	preserved := make(map[uint32]bool)

	s := newSharky(t, dir, shards, datasize)
	for i := range locs {
		binary.BigEndian.PutUint32(data, uint32(i))
		loc, err := s.Write(ctx, data)
		if err != nil {
			t.Fatal(err)
		}
		locs[i] = loc
	}
	// extract locations to preserve / free in map
	indexes := make([]uint32, size)
	for i := range indexes {
		indexes[i] = uint32(i)
	}
	rest := indexes[:]
	for n := size; n > size/2; n-- {
		i := rand.Intn(n)
		preserved[rest[i]] = false
		rest = append(rest[:i], rest[i+1:]...)
	}
	if len(rest) != len(preserved) {
		t.Fatalf("incorrect set sizes: %d <> %d", len(rest), len(preserved))
	}
	for _, i := range rest {
		preserved[i] = true
	}

	t.Run("recover based on preserved map", func(t *testing.T) {
		r, err := sharky.NewRecovery(dir, shards, datasize)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
		})
		for i, add := range preserved {
			if add {
				if err := r.Add(locs[i]); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := r.Save(); err != nil {
			t.Fatal(err)
		}
	})

	payload := []byte{0xff}

	t.Run("check integrity of recovered sharky", func(t *testing.T) {
		s := newSharky(t, dir, shards, datasize)
		buf := make([]byte, datasize)
		t.Run("preserved are found", func(t *testing.T) {
			for i := range preserved {
				loc := locs[i]
				if err := s.Read(ctx, loc, buf); err != nil {
					t.Fatal(err)
				}
				j := binary.BigEndian.Uint32(buf)
				if i != j {
					t.Fatalf("data not preserved at location %v: want %d; got %d", loc, i, j)
				}
			}
		})

		var freelocs []sharky.Location

		t.Run("correct number of free slots", func(t *testing.T) {
			s := newSharky(t, dir, 1, datasize)
			cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
			defer cancel()

			runs := 96
			for i := 0; i < runs; i++ {
				loc, err := s.Write(cctx, payload)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						break
					}
					t.Fatal(err)
				}
				freelocs = append(freelocs, loc)
			}
			if len(freelocs) != runs {
				t.Fatalf("incorrect number of free slots: wanted %d; got %d", runs, len(freelocs))
			}
		})
		t.Run("added locs are still preserved", func(t *testing.T) {
			for i, added := range preserved {
				if !added {
					continue
				}
				if err := s.Read(ctx, locs[int(i)], buf); err != nil {
					t.Fatal(err)
				}
				j := binary.BigEndian.Uint32(buf)
				if i != j {
					t.Fatalf("data not preserved at location %v: want %d; got %d", locs[int(j)], i, j)
				}
			}
		})
		t.Run("all other slots also overwritten", func(t *testing.T) {
			for _, loc := range freelocs {
				if err := s.Read(ctx, loc, buf); err != nil {
					t.Fatal(err)
				}
				data := buf[:len(payload)]
				if !bytes.Equal(data, payload) {
					t.Fatalf("incorrect data on freed location %v: want %x; got %x", loc, payload, data)
				}
			}
		})
	})
}

func newSharky(t *testing.T, dir string, shards, datasize int) *sharky.Store {
	t.Helper()
	s, err := sharky.New(&dirFS{basedir: dir}, shards, datasize)
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

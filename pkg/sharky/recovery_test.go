// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

func TestMissingShard(t *testing.T) {
	t.Parallel()

	_, err := sharky.NewRecovery(t.TempDir(), 1, 8)
	if !errors.Is(err, sharky.ErrShardNotFound) {
		t.Fatalf("want %v, got %v", sharky.ErrShardNotFound, err)
	}
}

// TestRecoveryShardOutOfRange checks that a persisted Shard value at or beyond
// the shard count is rejected instead of panicking with index out of range, as
// recovery locations decode from the same on-disk records as Store locations.
func TestRecoveryShardOutOfRange(t *testing.T) {
	t.Parallel()

	const shards = 2
	dir := t.TempDir()
	newSharky(t, dir, shards, 8)

	r, err := sharky.NewRecovery(dir, shards, 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	})

	ctx := context.Background()
	// Shard index equal to the shard count is one past the last valid shard.
	loc := sharky.Location{Shard: uint8(shards), Slot: 0, Length: 1}

	if err := r.Add(loc); !errors.Is(err, sharky.ErrShardNotFound) {
		t.Fatalf("Add: expected %v, got %v", sharky.ErrShardNotFound, err)
	}
	if err := r.Read(ctx, loc, make([]byte, 1)); !errors.Is(err, sharky.ErrShardNotFound) {
		t.Fatalf("Read: expected %v, got %v", sharky.ErrShardNotFound, err)
	}
	if err := r.Move(ctx, loc, sharky.Location{}); !errors.Is(err, sharky.ErrShardNotFound) {
		t.Fatalf("Move from: expected %v, got %v", sharky.ErrShardNotFound, err)
	}
	if err := r.Move(ctx, sharky.Location{}, loc); !errors.Is(err, sharky.ErrShardNotFound) {
		t.Fatalf("Move to: expected %v, got %v", sharky.ErrShardNotFound, err)
	}
	if err := r.TruncateAt(ctx, uint8(shards), 0); !errors.Is(err, sharky.ErrShardNotFound) {
		t.Fatalf("TruncateAt: expected %v, got %v", sharky.ErrShardNotFound, err)
	}
}

// nolint:paralleltest
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
			for range runs {
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

// TestRecoveryFilePermissions verifies that the free slot files recreated on the
// recovery path are not readable by other users on the host. This path bypasses
// the store's own file opener, so without it the tightened permissions would be
// reverted on every unclean shutdown.
func TestRecoveryFilePermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on windows")
	}

	const shards = 2

	dir := t.TempDir()
	s, err := sharky.New(&dirFS{basedir: dir}, shards, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(context.Background(), []byte("test")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// remove the free files so that recovery has to recreate them.
	for i := range shards {
		if err := os.Remove(path.Join(dir, fmt.Sprintf("free_%03d", i))); err != nil {
			t.Fatal(err)
		}
	}

	r, err := sharky.NewRecovery(dir, shards, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	for i := range shards {
		name := path.Join(dir, fmt.Sprintf("free_%03d", i))
		fi, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != 0o600 {
			t.Errorf("%s: got mode %O, want %O", name, got, 0o600)
		}
	}
}

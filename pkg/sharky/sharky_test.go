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
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
)

func TestSingleRetrieval(t *testing.T) {
	defer func(c int64) { sharky.DataSize = c }(sharky.DataSize)
	sharky.DataSize = 4

	dir := t.TempDir()
	s, err := sharky.New(dir, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	t.Run("write and read", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			want []byte
			err  error
		}{
			{
				"short data",
				[]byte{0x1},
				nil,
			}, {
				"exact size data",
				[]byte{1, 1, 1, 1},
				nil,
			}, {
				"exact size data 2",
				[]byte{1, 1, 1, 1},
				nil,
			}, {
				"long data",
				[]byte("long data"),
				sharky.ErrTooLong,
			}, {
				"exact size data 3",
				[]byte{1, 1, 1, 1},
				nil,
			}, {
				"capacity reached",
				[]byte{0x1},
				context.DeadlineExceeded,
			},
		} {

			t.Run(tc.name, func(t *testing.T) {
				cctx, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				loc, err := s.Write(cctx, tc.want)
				if !errors.Is(err, tc.err) {
					t.Fatalf("error mismatch on write. want %v, got %v", tc.err, err)
				}
				if err != nil {
					return
				}
				got, err := s.Read(ctx, loc)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(tc.want, got) {
					t.Fatalf("data mismatch at location %v. want %x, got %x", loc, tc.want, got)
				}
			})
		}
	})
}

// TestPersistence tests behaviour across several process sessions
// and checks if items and pregenerated free slots are persisted correctly
func TestPersistence(t *testing.T) {
	defer func(c int64) { sharky.DataSize = c }(sharky.DataSize)
	sharky.DataSize = 4
	shards := 4
	limit := int64(16)
	items := shards * int(limit)

	dir := t.TempDir()
	buf := make([]byte, 4)
	locs := make([]sharky.Location, items)
	i := 0
	ctx := context.Background()

	// simulate several subsequent sessions filling up the store
	for j := 0; i < items; j++ {
		s, err := sharky.New(dir, shards, limit)
		if err != nil {
			t.Fatal(err)
		}
		for ; i < items && rand.Intn(4) > 0; i++ {
			binary.BigEndian.PutUint32(buf, uint32(i))
			loc, err := s.Write(ctx, buf)
			if err != nil {
				t.Fatal(err)
			}
			locs[i] = loc
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// check location and data consisency
	s, err := sharky.New(dir, shards, limit)
	if err != nil {
		t.Fatal(err)
	}
	for want, loc := range locs {
		data, err := s.Read(ctx, loc)
		if err != nil {
			t.Fatal(err)
		}
		got := binary.BigEndian.Uint32(data)
		if int(got) != want {
			t.Fatalf("data mismatch. want %d, got %d", want, got)
		}
	}

	// the store has no more capacity, write expected to time out on waiting for free slots
	cctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, err = s.Write(cctx, []byte{0})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected error DeadlineExceeded, got %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRelease(t *testing.T) {
	defer func(c int64) { sharky.DataSize = c }(sharky.DataSize)
	sharky.DataSize = 4
	shards := 3
	limit := int64(1024)

	dir := t.TempDir()
	locs := make([][]sharky.Location, 4)
	ctx := context.Background()
	s, err := sharky.New(dir, shards, limit)
	if err != nil {
		t.Fatal(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(4)
	trigger := func(k int) {
		defer wg.Done()
		locs[k] = make([]sharky.Location, limit)
		for i := k * int(limit); i < (k+1)*int(limit); i++ {
			buf := make([]byte, 4)
			if i%4 == 3 {
				s.Release(ctx, locs[k][(i-1)%int(limit)])
			}
			binary.BigEndian.PutUint32(buf, uint32(i))
			loc, err := s.Write(ctx, buf)
			if err != nil {
				t.Error(err)
				return
			}
			locs[k][i%int(limit)] = loc
		}
	}
	for k := 0; k < 4; k++ {
		go trigger(k)
	}

	wg.Wait()
	for k := 0; k < 4; k++ {
		for i, loc := range locs[k] {
			if i%4 == 2 {
				continue
			}
			want := i + k*int(limit)
			data, err := s.Read(ctx, loc)
			if err != nil {
				t.Fatal(err)
			}
			got := int(binary.BigEndian.Uint32(data))
			if got != want {
				t.Fatalf("data mismatch. want %d, got %d", want, got)
			}
		}
	}
	// the store has no more capacity, write expected to time out on waiting for free slots
	cctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, err = s.Write(cctx, []byte{0})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected error DeadlineExceeded, got %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

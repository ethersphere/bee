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
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
	"golang.org/x/sync/errgroup"
)

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
}

func TestSingleRetrieval(t *testing.T) {
	datasize := 4
	dir := t.TempDir()
	s, err := sharky.New(&dirFS{basedir: dir}, 2, datasize)
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
			},
		} {
			buf := make([]byte, datasize)
			t.Run(tc.name, func(t *testing.T) {
				cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
				defer cancel()
				loc, err := s.Write(cctx, tc.want)
				if !errors.Is(err, tc.err) {
					t.Fatalf("error mismatch on write. want %v, got %v", tc.err, err)
				}
				if err != nil {
					return
				}
				err = s.Read(ctx, loc, buf)
				if err != nil {
					t.Fatal(err)
				}
				got := buf[:loc.Length]
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
	datasize := 4
	shards := 2
	shardSize := uint32(16)
	items := shards * int(shardSize)

	dir := t.TempDir()
	buf := make([]byte, 4)
	locs := make([]*sharky.Location, items)
	i := 0
	j := 0
	ctx := context.Background()
	// simulate several subsequent sessions filling up the store
	for ; i < items; j++ {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		s, err := sharky.New(&dirFS{basedir: dir}, shards, datasize)
		if err != nil {
			t.Fatal(err)
		}
		for ; i < items && rand.Intn(4) > 0; i++ {
			if locs[i] != nil {
				continue
			}
			binary.BigEndian.PutUint32(buf, uint32(i))
			loc, err := s.Write(cctx, buf)
			if err != nil {
				t.Fatal(err)
			}
			locs[i] = &loc
		}
		cancel()
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("got full in %d sessions\n", j)

	// check location and data consisency
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	s, err := sharky.New(&dirFS{basedir: dir}, shards, datasize)
	if err != nil {
		t.Fatal(err)
	}
	buf = make([]byte, datasize)
	j = 0
	for want, loc := range locs {
		j++
		err := s.Read(cctx, *loc, buf)
		if err != nil {
			t.Fatal(err)
		}
		got := binary.BigEndian.Uint32(buf)
		if int(got) != want {
			t.Fatalf("data mismatch. want %d, got %d", want, got)
		}
	}
	cancel()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrency(t *testing.T) {
	datasize := 4
	test := func(t *testing.T, workers, shards int, shardSize uint32) {
		limit := shards * int(shardSize)

		dir := t.TempDir()
		defer os.RemoveAll(dir)
		s, err := sharky.New(&dirFS{basedir: dir}, shards, datasize)
		if err != nil {
			t.Fatal(err)
		}
		c := make(chan sharky.Location, limit)
		start := make(chan struct{})
		deleted := make(map[uint32]int)
		entered := make(map[uint32]struct{})
		ctx := context.Background()
		eg, ectx := errgroup.WithContext(ctx)
		// a number of workers write sequential numbers to sharky
		for k := 0; k < workers; k++ {
			k := k
			eg.Go(func() error {
				<-start
				buf := make([]byte, 4)
				for i := 0; i < limit; i++ {
					j := i*workers + k
					binary.BigEndian.PutUint32(buf, uint32(j))
					loc, err := s.Write(ctx, buf)
					if err != nil {
						return err
					}
					select {
					case <-ectx.Done():
						return ectx.Err()
					case c <- loc:
					}
				}
				return nil
			})
		}
		// parallel to these workers, other workers collect the taken slots and release them
		// modelling some aggressive gc policy
		mtx := sync.Mutex{}
		for k := 0; k < workers-1; k++ {
			eg.Go(func() error {
				<-start
				buf := make([]byte, datasize)
				for i := 0; i < limit; i++ {
					select {
					case <-ectx.Done():
						return ectx.Err()
					case loc := <-c:
						if err := s.Read(ectx, loc, buf); err != nil {
							return err
						}
						j := binary.BigEndian.Uint32(buf)
						mtx.Lock()
						deleted[j]++
						mtx.Unlock()
						if err := s.Release(ectx, loc); err != nil {
							return err
						}
					}
				}
				return nil
			})
		}
		close(start)
		if err := eg.Wait(); err != nil {
			t.Fatal(err)
		}
		close(c)
		extraSlots := 0
		for i := uint32(0); i < uint32(workers*limit); i++ {
			cnt, found := deleted[i]
			if !found {
				entered[i] = struct{}{}
				continue
			}
			extraSlots += cnt - 1
		}
		buf := make([]byte, datasize)
		for loc := range c {
			err := s.Read(ctx, loc, buf)
			if err != nil {
				t.Error(err)
				return
			}
			i := binary.BigEndian.Uint32(buf)

			_, found := entered[i]
			if !found {
				t.Fatal("item at unreleased location incorrect")
			}
		}

		// the store has extra slots capacity
		cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		for i := 0; i < extraSlots; i++ {
			t.Logf("checking extra slot %d\n", i)
			_, err = s.Write(cctx, []byte{0})
			if err != nil {
				t.Fatal(err)
			}
		}
		cancel()

		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		workers, shards int
		shardSize       uint32
	}{
		{3, 2, 2},
		{2, 64, 2},
		{32, 8, 32},
		{64, 32, 64},
	} {
		t.Run(fmt.Sprintf("workers:%d,shards:%d,size:%d", c.workers, c.shards, c.shardSize), func(t *testing.T) {
			test(t, c.workers, c.shards, c.shardSize)
		})
	}
}

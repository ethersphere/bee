// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package sharky

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestShard ensures that released slots eventually become available for writes
func TestShard(t *testing.T) {
	t.Parallel()

	shard := newShard(t)

	write := []byte{0xff}
	loc := writePayload(t, shard, write)
	buf := readFromLocation(t, shard, loc)

	if !bytes.Equal(buf, write) {
		t.Fatalf("want %x, got %x", buf, write)
	}

	if loc.Slot != 0 {
		t.Fatalf("expected to write to slot 0, got %d", loc.Slot)
	}

	// in order for the test to succeed this slot is expected to become available before test finishes
	shard.release(loc.Slot)

	write = []byte{0xff >> 1}
	loc = writePayload(t, shard, write)

	// immediate write should pick the next slot
	if loc.Slot != 1 {
		t.Fatalf("expected to write to slot 1, got %d", loc.Slot)
	}

	shard.release(loc.Slot)

	// we make ten writes expecting that slot 0 is released and becomes available for writing eventually
	i, runs := 0, 10
	for ; i < runs; i++ {
		write = []byte{0x01 << i}
		loc = writePayload(t, shard, write)
		shard.release(loc.Slot)
		if loc.Slot == 0 {
			break
		}
	}

	if i == runs {
		t.Errorf("expected to write to slot 0 within %d runs, write did not occur", runs)
	}
}

func writePayload(t *testing.T, shard *shard, buf []byte) Location {
	t.Helper()

	select {
	case slot := <-shard.available:
		loc, err := shard.write(buf, slot.slot)
		if err != nil {
			t.Fatal(err)
		}
		return loc
	case <-time.After(100 * time.Millisecond):
		t.Fatal("write timeout")
	}

	return Location{}
}

func readFromLocation(t *testing.T, shard *shard, loc Location) []byte {
	t.Helper()
	buf := make([]byte, loc.Length)

	err := shard.read(buf, loc.Slot)
	if err != nil {
		t.Fatal(err)
	}

	return buf
}

type dirFS string

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(string(*d), path), os.O_RDWR|os.O_CREATE, 0644)
}

func newShard(t *testing.T) *shard {
	t.Helper()

	basedir := dirFS(t.TempDir())
	index := 1

	file, err := basedir.Open(fmt.Sprintf("shard_%03d", index))
	if err != nil {
		t.Fatal(err)
	}

	ffile, err := basedir.Open(fmt.Sprintf("free_%03d", index))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	slots := newSlots(ffile.(sharkyFile))
	err = slots.load()
	if err != nil {
		t.Fatal(err)
	}

	quit := make(chan struct{})
	shard := &shard{
		available:   make(chan availableShard),
		index:       uint8(index),
		maxDataSize: 1,
		file:        file.(sharkyFile),
		slots:       slots,
		quit:        quit,
	}

	t.Cleanup(func() {
		close(quit)
		if err := shard.close(); err != nil {
			t.Fatal("close shard", err)
		}
	})

	terminated := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		shard.process()
		close(terminated)
	}()

	return shard
}

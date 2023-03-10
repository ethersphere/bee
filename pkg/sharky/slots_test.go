// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky

import (
	"bytes"
	"testing"
)

func TestFreeSlotsPersist(t *testing.T) {
	t.Parallel()

	basedir := dirFS(t.TempDir())

	freeFileName := "free_000"

	ffile, err := basedir.Open(freeFileName)
	if err != nil {
		t.Fatal(err)
	}

	slots := newSlots(ffile.(sharkyFile))
	err = slots.Load()
	if err != nil {
		t.Fatal(err)
	}

	// use 80 slots
	for i := uint32(0); i < 80; i++ {
		slots.Next()
	}

	// 8 bytes should be all zero, meaning 80 bits all used
	if !bytes.Equal(slots.data[:8], make([]byte, 8)) {
		t.Fatal("data mismatch")
	}

	// grab 80th free slot
	nextFree := slots.Next()
	if nextFree != 80 {
		t.Fatalf("want slot 80, got %d", nextFree)
	}

	// free a subsection
	for i := uint32(10); i < 20; i++ {
		slots.Free(i)
	}

	nextFree = slots.Next()
	if nextFree != 10 {
		t.Fatalf("want slot 10, got %d", nextFree)
	}

	// save and close file
	err = slots.Close()
	if err != nil {
		t.Fatal(err)
	}

	ffile, err = basedir.Open(freeFileName)
	if err != nil {
		t.Fatal(err)
	}

	slots = newSlots(ffile.(sharkyFile))
	err = slots.Load()
	if err != nil {
		t.Fatal(err)
	}

	// check previous free slots were persisted
	// and Use fills the free subsection
	for i := uint32(11); i < 20; i++ {
		n := slots.Next()
		if n != i {
			t.Fatalf("want slot %d, got %d", i, n)
		}
	}

	if !bytes.Equal(slots.data[:8], make([]byte, 8)) {
		t.Fatal("data mismatch")
	}
}

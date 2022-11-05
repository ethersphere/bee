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
	var err error

	ffile, err := basedir.Open("free_000")
	if err != nil {
		t.Fatal(err)
	}

	slot := newSlots(ffile.(sharkyFile))
	err = slot.load()
	if err != nil {
		t.Fatal(err)
	}

	// use 80 slots
	for i := uint32(0); i < 80; i++ {
		slot.Use(slot.Next())
	}

	// 8 bytes should be all zero, meaning 80 bits all used
	if !bytes.Equal(slot.data[:8], make([]byte, 8)) {
		t.Fatal("data mismatch")
	}

	// grab 80th free slot
	nextFree := slot.Next()
	if nextFree != 80 {
		t.Fatalf("want slot 80, got %d", nextFree)
	}

	// free a subsection
	for i := uint32(10); i < 20; i++ {
		slot.Free(i)
	}

	nextFree = slot.Next()
	if nextFree != 10 {
		t.Fatalf("want slot 10, got %d", nextFree)
	}

	// save and close file
	err = slot.Save()
	if err != nil {
		t.Fatal(err)
	}
	err = ffile.Close()
	if err != nil {
		t.Fatal(err)
	}

	ffile, err = basedir.Open("free_000")
	if err != nil {
		t.Fatal(err)
	}

	slot = newSlots(ffile.(sharkyFile))
	err = slot.load()
	if err != nil {
		t.Fatal(err)
	}

	// check previous free slots were persisted
	// and Use fills the free subsection
	for i := uint32(10); i < 20; i++ {
		n := slot.Next()
		if n != i {
			t.Fatalf("want slot %d, got %d", i, n)
		}
		slot.Use(n)
	}

	if !bytes.Equal(slot.data[:8], make([]byte, 8)) {
		t.Fatal("data mismatch")
	}
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestChunkPipe verifies that the reads are correctly buffered for
// two unaligned writes across two chunks.
func TestChunkPipe(t *testing.T) {
	buf := file.NewChunkPipe()

	errC := make(chan error)
	go func() {
		data := make([]byte, swarm.ChunkSize)
		c, err := buf.Read(data)
		if err != nil {
			errC <- err
		}
		if c != swarm.ChunkSize {
			errC <- fmt.Errorf("short read %d", c)
		}
		c, err = buf.Read(data)
		if c != 2 {
			errC <- fmt.Errorf("read expected 2, got %d", c)
		}
		if err != nil {
			errC <- err
		}
		errC <- nil
	}()
	data := [swarm.ChunkSize - 2]byte{}
	c, err := buf.Write(data[:])
	if err != nil {
		t.Fatal(err)
	}
	if c != len(data) {
		t.Fatalf("short write")
	}
	c, err = buf.Write(data[:4])
	if err != nil {
		t.Fatal(err)
	}
	if c != 4 {
		t.Fatalf("short write")
	}

	err = buf.Close()
	if err != nil {
		t.Fatal(err)
	}

	timer := time.NewTimer(time.Millisecond)
	select {
	case err = <-errC:
	case <-timer.C:
		t.Fatal("timeout")
	}
	if err != nil {
		t.Fatal(err)
	}
}

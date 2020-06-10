// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	dataWrites = [][]int{
		{swarm.ChunkSize - 2},                         // short
		{swarm.ChunkSize - 2, 4},                      // short, over
		{swarm.ChunkSize - 2, 4, swarm.ChunkSize - 6}, // short, over, short
		{swarm.ChunkSize - 2, 4, swarm.ChunkSize - 4}, // short, over, onononon
		{swarm.ChunkSize, 2, swarm.ChunkSize - 4},     // on, short, short
		{swarm.ChunkSize, 2, swarm.ChunkSize - 2},     // on, short, on
		{swarm.ChunkSize, 2, swarm.ChunkSize},         // on, short, over
		{swarm.ChunkSize, 2, swarm.ChunkSize - 2, 4},  // on, short, on, short
		{swarm.ChunkSize, swarm.ChunkSize},            // on, on
	}
)

// TestChunkPipe verifies that the reads are correctly buffered for
// various write length combinations.
func TestChunkPipe(t *testing.T) {
	for i := range dataWrites {
		t.Run(fmt.Sprintf("%d", i), testChunkPipe)
	}
}

func testChunkPipe(t *testing.T) {
	paramString := strings.Split(t.Name(), "/")
	dataWriteIdx, err := strconv.ParseInt(paramString[1], 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	buf := file.NewChunkPipe()

	sizeC := make(chan int, 255)
	errC := make(chan error, 1)
	go func() {
		data := make([]byte, swarm.ChunkSize)
		for {
			// get buffered chunkpipe read
			c, err := buf.Read(data)
			sizeC <- c
			if err != nil {
				close(sizeC)
				errC <- err
				return
			}

			// only the last read should be smaller than chunk size
			if c < swarm.ChunkSize {
				close(sizeC)
				errC <- nil
				return
			}
		}
	}()

	// do the writes
	dataWrite := dataWrites[dataWriteIdx]
	writeTotal := 0
	for _, l := range dataWrite {
		data := make([]byte, l)
		c, err := buf.Write(data)
		if err != nil {
			t.Fatal(err)
		}
		if c != l {
			t.Fatalf("short write")
		}
		writeTotal += l
	}

	// finish up (last unfinished chunk write will be flushed)
	err = buf.Close()
	if err != nil {
		t.Fatal(err)
	}

	// receive the writes
	// err may or may not be EOF, depending on whether writes end on
	// chunk boundary
	timer := time.NewTimer(time.Second)
	readTotal := 0
OUTER:
	for {
		select {
		case c := <-sizeC:
			readTotal += c
		case err = <-errC:
			if err != nil {
				if err != io.EOF {
					t.Fatal(err)
				}
			}
			break OUTER
		case <-timer.C:
			t.Fatal("timeout")
		}
	}

	// check that the write amounts match
	if readTotal != writeTotal {
		t.Fatalf("expected read %d, got %d", readTotal, writeTotal)
	}

}

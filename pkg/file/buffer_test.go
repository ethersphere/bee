// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
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
	for {
		select {
		case c := <-sizeC:
			readTotal += c
			if readTotal == writeTotal {
				return
			}
		case err = <-errC:
			if err != nil {
				if err != io.EOF {
					t.Fatal(err)
				}
			}
		case <-timer.C:
			t.Fatal("timeout")
		}
	}
}

func TestCopyBuffer(t *testing.T) {
	readBufferSizes := []int{
		64,
		1024,
		swarm.ChunkSize,
	}
	dataSizes := []int{
		1,
		64,
		1024,
		swarm.ChunkSize - 1,
		swarm.ChunkSize,
		swarm.ChunkSize + 1,
		swarm.ChunkSize * 2,
		swarm.ChunkSize*2 + 3,
		swarm.ChunkSize * 5,
		swarm.ChunkSize*5 + 3,
		swarm.ChunkSize * 17,
		swarm.ChunkSize*17 + 3,
	}

	testCases := []struct {
		readBufferSize int
		dataSize       int
	}{}

	for i := 0; i < len(readBufferSizes); i++ {
		for j := 0; j < len(dataSizes); j++ {
			testCases = append(testCases, struct {
				readBufferSize int
				dataSize       int
			}{readBufferSizes[i], dataSizes[j]})
		}
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("buf_%-4d/data_size_%d", tc.readBufferSize, tc.dataSize), func(t *testing.T) {
			// https://golang.org/doc/faq#closures_and_goroutines
			readBufferSize := tc.readBufferSize
			dataSize := tc.dataSize

			srcBytes := make([]byte, dataSize)

			rand.Read(srcBytes)

			chunkPipe := file.NewChunkPipe()

			// destination
			sizeC := make(chan int)
			dataC := make(chan []byte)
			go reader(t, readBufferSize, chunkPipe, sizeC, dataC)

			// source
			errC := make(chan error, 1)
			go func() {
				src := bytes.NewReader(srcBytes)

				buf := make([]byte, swarm.ChunkSize)
				c, err := io.CopyBuffer(chunkPipe, src, buf)
				if err != nil {
					errC <- err
				}

				if c != int64(dataSize) {
					errC <- errors.New("read count mismatch")
				}

				err = chunkPipe.Close()
				if err != nil {
					errC <- err
				}

				close(errC)
			}()

			// receive the writes
			// err may or may not be EOF, depending on whether writes end on
			// chunk boundary
			expected := dataSize
			timer := time.NewTimer(time.Second)
			readTotal := 0
			readData := []byte{}
			for {
				select {
				case c := <-sizeC:
					readTotal += c
					if readTotal == expected {

						// check received content
						if !bytes.Equal(srcBytes, readData) {
							t.Fatal("invalid byte content received")
						}

						return
					}
				case d := <-dataC:
					readData = append(readData, d...)
				case err := <-errC:
					if err != nil {
						if err != io.EOF {
							t.Fatal(err)
						}
					}
				case <-timer.C:
					t.Fatal("timeout")
				}
			}
		})
	}
}

func reader(t *testing.T, bufferSize int, r io.Reader, c chan int, cd chan []byte) {
	var buf = make([]byte, bufferSize)
	for {
		n, err := r.Read(buf)
		if err == io.EOF {
			c <- 0
			break
		}
		if err != nil {
			t.Errorf("read: %v", err)
		}

		b := make([]byte, n)
		copy(b, buf)
		cd <- b

		c <- n
	}
}

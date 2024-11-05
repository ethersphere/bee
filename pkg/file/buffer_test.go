// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// TestChunkPipe verifies that the reads are correctly buffered for
// various write length combinations.
func TestChunkPipe(t *testing.T) {
	t.Parallel()

	dataWrites := [][]int{
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
	for i, tc := range dataWrites {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			buf := file.NewChunkPipe()
			readResultC := make(chan readResult, 16)
			go func() {
				data := make([]byte, swarm.ChunkSize)
				for {
					// get buffered chunkpipe read
					n, err := buf.Read(data)
					readResultC <- readResult{n: n, err: err}

					// only the last read should be smaller than chunk size
					if n < swarm.ChunkSize {
						return
					}
				}
			}()

			// do the writes
			writeTotal := 0
			for _, l := range tc {
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
			err := buf.Close()
			if err != nil {
				t.Fatal(err)
			}

			// receive the writes
			// err may or may not be EOF, depending on whether writes end on
			// chunk boundary
			readTotal := 0
			for res := range readResultC {
				if res.err != nil && !errors.Is(res.err, io.EOF) {
					t.Fatal(res.err)
				}

				readTotal += res.n
				if readTotal == writeTotal {
					return
				}
			}
		})
	}
}

func TestCopyBuffer(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			readBufferSize := tc.readBufferSize
			dataSize := tc.dataSize
			chunkPipe := file.NewChunkPipe()
			srcBytes := testutil.RandBytes(t, dataSize)

			// destination
			resultC := make(chan readResult, 1)
			go reader(t, readBufferSize, chunkPipe, resultC)

			// source
			errC := make(chan error, 16)
			go func() {
				src := bytes.NewReader(srcBytes)

				buf := make([]byte, swarm.ChunkSize)
				c, err := io.CopyBuffer(chunkPipe, src, buf)
				errC <- err

				if c != int64(dataSize) {
					errC <- errors.New("read count mismatch")
				}

				errC <- chunkPipe.Close()
			}()

			// receive the writes
			// err may or may not be EOF, depending on whether writes end on chunk boundary
			readData := []byte{}
			for {
				select {
				case res, ok := <-resultC:
					if !ok {
						// when resultC is closed (there is no more data to read)
						// assert if read data is same as source
						if !bytes.Equal(srcBytes, readData) {
							t.Fatal("invalid byte content received")
						}

						return
					}

					readData = append(readData, res.data...)
				case err := <-errC:
					if err != nil && !errors.Is(err, io.EOF) {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

type readResult struct {
	data []byte
	n    int
	err  error
}

func reader(t *testing.T, bufferSize int, r io.Reader, c chan<- readResult) {
	t.Helper()

	defer close(c)

	buf := make([]byte, bufferSize)
	for {
		n, err := r.Read(buf)
		if errors.Is(err, io.EOF) {
			c <- readResult{n: n}
			return
		}

		if err != nil {
			t.Errorf("read: %v", err)
		}

		data := make([]byte, n)
		copy(data, buf)

		c <- readResult{n: n, data: data}
	}
}

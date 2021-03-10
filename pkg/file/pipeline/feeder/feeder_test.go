// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feeder_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/feeder"
)

// TestFeeder tests that partial writes work correctly.
func TestFeeder(t *testing.T) {
	var (
		chunkSize = 5
		data      = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	)

	for _, tc := range []struct {
		name      string // name
		dataSize  []int  // how big each write is
		expWrites int    // expected number of writes
		writeData []byte // expected data in last write buffer
		span      uint64 // expected span of written data
	}{
		{
			name:      "empty write",
			dataSize:  []int{0},
			expWrites: 0,
		},
		{
			name:      "less than chunk, no writes",
			dataSize:  []int{3},
			expWrites: 0,
		},
		{
			name:      "one chunk, one write",
			dataSize:  []int{5},
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
			span:      5,
		},
		{
			name:      "two chunks, two writes",
			dataSize:  []int{10},
			expWrites: 2,
			writeData: []byte{6, 7, 8, 9, 10},
			span:      5,
		},
		{
			name:      "half chunk, then full one, one write",
			dataSize:  []int{3, 5},
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
			span:      5,
		},
		{
			name:      "half chunk, another two halves, one write",
			dataSize:  []int{3, 2, 3},
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
			span:      5,
		},
		{
			name:      "half chunk, another two halves, another full, two writes",
			dataSize:  []int{3, 2, 3, 5},
			expWrites: 2,
			writeData: []byte{6, 7, 8, 9, 10},
			span:      5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var results pipeline.PipeWriteArgs
			rr := newMockResultWriter(&results)
			cf := feeder.NewChunkFeederWriter(chunkSize, rr)
			i := 0
			for _, v := range tc.dataSize {
				d := data[i : i+v]
				n, err := cf.Write(d)
				if err != nil {
					t.Fatal(err)
				}
				if n != v {
					t.Fatalf("wrote %d bytes but expected %d bytes", n, v)
				}
				i += v
			}

			if tc.expWrites == 0 && results.Data != nil {
				t.Fatal("expected no write but got one")
			}

			if rr.count != tc.expWrites {
				t.Fatalf("expected %d writes but got %d", tc.expWrites, rr.count)
			}

			if results.Data != nil && !bytes.Equal(tc.writeData, results.Data[8:]) {
				t.Fatalf("expected write data %v but got %v", tc.writeData, results.Data[8:])
			}

			if tc.span > 0 {
				v := binary.LittleEndian.Uint64(results.Data[:8])
				if v != tc.span {
					t.Fatalf("span mismatch, got %d want %d", v, tc.span)
				}
			}
		})
	}
}

// TestFeederFlush tests that the feeder flushes the data in the buffer correctly
// on Sum().
func TestFeederFlush(t *testing.T) {
	var (
		chunkSize = 5
		data      = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	)

	for _, tc := range []struct {
		name      string // name
		dataSize  []int  // how big each write is
		expWrites int    // expected number of writes
		writeData []byte // expected data in last write buffer
		span      uint64 // expected span of written data
	}{
		{
			name:      "empty file",
			dataSize:  []int{0},
			expWrites: 1,
		},
		{
			name:      "less than chunk, one write",
			dataSize:  []int{3},
			expWrites: 1,
			writeData: []byte{1, 2, 3},
		},
		{
			name:      "one chunk, one write",
			dataSize:  []int{5},
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
			span:      5,
		},
		{
			name:      "two chunks, two writes",
			dataSize:  []int{10},
			expWrites: 2,
			writeData: []byte{6, 7, 8, 9, 10},
			span:      5,
		},
		{
			name:      "half chunk, then full one, two writes",
			dataSize:  []int{3, 5},
			expWrites: 2,
			writeData: []byte{6, 7, 8},
			span:      3,
		},
		{
			name:      "half chunk, another two halves, two writes",
			dataSize:  []int{3, 2, 3},
			expWrites: 2,
			writeData: []byte{6, 7, 8},
			span:      3,
		},
		{
			name:      "half chunk, another two halves, another full, three writes",
			dataSize:  []int{3, 2, 3, 5},
			expWrites: 3,
			writeData: []byte{11, 12, 13},
			span:      3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var results pipeline.PipeWriteArgs
			rr := newMockResultWriter(&results)
			cf := feeder.NewChunkFeederWriter(chunkSize, rr)
			i := 0
			for _, v := range tc.dataSize {
				d := data[i : i+v]
				n, err := cf.Write(d)
				if err != nil {
					t.Fatal(err)
				}
				if n != v {
					t.Fatalf("wrote %d bytes but expected %d bytes", n, v)
				}
				i += v
			}

			_, _ = cf.Sum()

			if tc.expWrites == 0 && results.Data != nil {
				t.Fatal("expected no write but got one")
			}

			if rr.count != tc.expWrites {
				t.Fatalf("expected %d writes but got %d", tc.expWrites, rr.count)
			}

			if results.Data != nil && !bytes.Equal(tc.writeData, results.Data[8:]) {
				t.Fatalf("expected write data %v but got %v", tc.writeData, results.Data[8:])
			}

			if tc.span > 0 {
				v := binary.LittleEndian.Uint64(results.Data[:8])
				if v != tc.span {
					t.Fatalf("span mismatch, got %d want %d", v, tc.span)
				}
			}
		})
	}
}

// countingResultWriter counts how many writes were done to it
// and passes the results to the caller using the pointer provided
// in the constructor.
type countingResultWriter struct {
	target *pipeline.PipeWriteArgs
	count  int
}

func newMockResultWriter(b *pipeline.PipeWriteArgs) *countingResultWriter {
	return &countingResultWriter{target: b}
}

func (w *countingResultWriter) ChainWrite(p *pipeline.PipeWriteArgs) error {
	w.count++
	*w.target = *p
	return nil
}

func (w *countingResultWriter) Sum() ([]byte, error) {
	return nil, errors.New("not implemented")
}

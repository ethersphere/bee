// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy_test

import (
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/bmt"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type mockEncoder struct {
	shards, parities int
}

func newMockEncoder(shards, parities int) (redundancy.ErasureEncoder, error) {
	return &mockEncoder{
		shards:   shards,
		parities: parities,
	}, nil
}

// Encode makes MSB of span equal to data
func (m *mockEncoder) Encode(buffer [][]byte) error {
	// writes parity data
	indicatedValue := 0
	for i := m.shards; i < m.shards+m.parities; i++ {
		data := make([]byte, 32)
		data[swarm.SpanSize-1], data[swarm.SpanSize] = uint8(indicatedValue), uint8(indicatedValue)
		buffer[i] = data
		indicatedValue++
	}
	return nil
}

type ParityChainWriter struct {
	sync.Mutex
	chainWriteCalls int
	sumCalls        int
	validCalls      []bool
}

func NewParityChainWriter() *ParityChainWriter {
	return &ParityChainWriter{}
}

func (c *ParityChainWriter) ChainWriteCalls() int {
	c.Lock()
	defer c.Unlock()
	return c.chainWriteCalls
}
func (c *ParityChainWriter) SumCalls() int { c.Lock(); defer c.Unlock(); return c.sumCalls }

func (c *ParityChainWriter) ChainWrite(args *pipeline.PipeWriteArgs) error {
	c.Lock()
	defer c.Unlock()
	valid := args.Span[len(args.Span)-1] == args.Data[len(args.Span)] && args.Data[len(args.Span)] == byte(c.chainWriteCalls)
	c.chainWriteCalls++
	c.validCalls = append(c.validCalls, valid)
	return nil
}
func (c *ParityChainWriter) Sum() ([]byte, error) {
	c.Lock()
	defer c.Unlock()
	c.sumCalls++
	return nil, nil
}

func TestEncode(t *testing.T) {
	t.Parallel()
	// initializes mockEncoder -> creates shard chunks -> redundancy.chunkWrites -> call encode
	erasureEncoder := redundancy.GetErasureEncoder()
	defer func() {
		redundancy.SetErasureEncoder(erasureEncoder)
	}()
	redundancy.SetErasureEncoder(newMockEncoder)

	// test on the data level
	for _, level := range []redundancy.Level{redundancy.MEDIUM, redundancy.STRONG, redundancy.INSANE, redundancy.PARANOID} {
		for _, encrypted := range []bool{false, true} {
			maxShards := level.GetMaxShards()
			if encrypted {
				maxShards = level.GetMaxEncShards()
			}
			for shardCount := 1; shardCount <= maxShards; shardCount++ {
				t.Run(fmt.Sprintf("redundancy level %d is checked with %d shards", level, shardCount), func(t *testing.T) {
					parityChainWriter := NewParityChainWriter()
					ppf := func() pipeline.ChainWriter {
						return bmt.NewBmtWriter(parityChainWriter)
					}
					params := redundancy.New(level, encrypted, ppf)
					// checks parity pipelinecalls are valid

					parityCount := 0
					parityCallback := func(level int, span, address []byte) error {
						parityCount++
						return nil
					}

					for i := 0; i < shardCount; i++ {
						buffer := make([]byte, 32)
						_, err := io.ReadFull(rand.Reader, buffer)
						if err != nil {
							t.Fatal(err)
						}
						err = params.ChunkWrite(0, buffer, parityCallback)
						if err != nil {
							t.Fatal(err)
						}
					}
					if shardCount != maxShards {
						// encode should be called automatically when reaching maxshards
						err := params.Encode(0, parityCallback)
						if err != nil {
							t.Fatal(err)
						}
					}

					// CHECKS

					if parityCount != parityChainWriter.chainWriteCalls {
						t.Fatalf("parity callback was called %d times meanwhile chainwrite was called %d times", parityCount, parityChainWriter.chainWriteCalls)
					}

					expectedParityCount := params.Level().GetParities(shardCount)
					if encrypted {
						expectedParityCount = params.Level().GetEncParities(shardCount)
					}
					if parityCount != expectedParityCount {
						t.Fatalf("parity callback was called %d times meanwhile expected parity number should be %d", parityCount, expectedParityCount)
					}

					for i, validCall := range parityChainWriter.validCalls {
						if !validCall {
							t.Fatalf("parity chunk data is wrong at parity index %d", i)
						}
					}
				})
			}
		}
	}
}

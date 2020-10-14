// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/bmt"
	mock "github.com/ethersphere/bee/pkg/file/pipeline/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestStoreWriter tests that store writer stores the provided data and calls the next chain writer.
func TestBmtWriter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		expHash []byte
		expErr  error
		noSpan  bool
	}{
		{
			// this is a special case, since semantically it can be considered the hash
			// of an empty file (since data is all zeros).
			name:    "all zeros",
			data:    make([]byte, swarm.ChunkSize),
			expHash: mustDecodeString(t, "09ae927d0f3aaa37324df178928d3826820f3dd3388ce4aaebfc3af410bde23a"),
		},
		{
			name:    "hello world",
			data:    []byte("hello world"),
			expHash: mustDecodeString(t, "92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f"),
		},
		{
			name:   "no data",
			data:   []byte{},
			noSpan: true,
			expErr: bmt.ErrInvalidData,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockChainWriter := mock.NewChainWriter()
			writer := bmt.NewBmtWriter(mockChainWriter)

			var data []byte

			if !tc.noSpan {
				data = make([]byte, 8)
				binary.LittleEndian.PutUint64(data, uint64(len(tc.data)))
			}

			data = append(data, tc.data...)
			args := pipeline.PipeWriteArgs{Data: data}

			err := writer.ChainWrite(&args)
			if err != nil && tc.expErr != nil && errors.Is(err, tc.expErr) {
				return
			}

			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tc.expHash, args.Ref) {
				t.Fatalf("ref mismatch. got %v want %v", args.Ref, tc.expHash)
			}

			if calls := mockChainWriter.ChainWriteCalls(); calls != 1 {
				t.Errorf("wanted 1 ChainWrite call, got %d", calls)
			}
		})
	}
}

// TestSum tests that calling Sum on the writer calls the next writer's Sum.
func TestSum(t *testing.T) {
	mockChainWriter := mock.NewChainWriter()
	writer := bmt.NewBmtWriter(mockChainWriter)
	_, err := writer.Sum()
	if err != nil {
		t.Fatal(err)
	}
	if calls := mockChainWriter.SumCalls(); calls != 1 {
		t.Fatalf("wanted 1 Sum call but got %d", calls)
	}
}

func mustDecodeString(t *testing.T, s string) []byte {
	t.Helper()
	v, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}

	return v
}

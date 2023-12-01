// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/pkg/storage"
	inmem "github.com/ethersphere/bee/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

func initData(ctx context.Context, erasureBuffer [][]byte, dataShardCount int, s storage.ChunkStore) ([]swarm.Address, []swarm.Address, error) {
	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, swarm.ChunkSize)
	for i := 0; i < dataShardCount; i++ {
		erasureBuffer[i] = make([]byte, swarm.HashSize)
		_, err := io.ReadFull(rand.Reader, erasureBuffer[i])
		if err != nil {
			return nil, nil, err
		}
		copy(erasureBuffer[i], spanBytes)
	}
	// make parity shards
	for i := dataShardCount; i < len(erasureBuffer); i++ {
		erasureBuffer[i] = make([]byte, swarm.HashSize)
	}
	// generate parity chunks
	rs, err := reedsolomon.New(dataShardCount, len(erasureBuffer)-dataShardCount)
	if err != nil {
		return nil, nil, err
	}
	err = rs.Encode(erasureBuffer)
	if err != nil {
		return nil, nil, err
	}
	// calculate chunk addresses and upload to the store
	var sAddresses, pAddresses []swarm.Address
	for i := 0; i < dataShardCount; i++ {
		chunk, err := cac.NewWithDataSpan(erasureBuffer[i])
		if err != nil {
			return nil, nil, err
		}
		err = s.Put(ctx, chunk)
		if err != nil {
			return nil, nil, err
		}
		sAddresses = append(sAddresses, chunk.Address())
	}
	for i := dataShardCount; i < len(erasureBuffer); i++ {
		chunk, err := cac.NewWithDataSpan(erasureBuffer[i])
		if err != nil {
			return nil, nil, err
		}
		err = s.Put(ctx, chunk)
		if err != nil {
			return nil, nil, err
		}
		pAddresses = append(pAddresses, chunk.Address())
	}

	return sAddresses, pAddresses, err
}

func dataShardsAvailable(ctx context.Context, sAddresses []swarm.Address, s storage.ChunkStore) error {
	for i := 0; i < len(sAddresses); i++ {
		has, err := s.Has(ctx, sAddresses[i])
		if err != nil {
			return err
		}
		if !has {
			return fmt.Errorf("datashard %d is not available", i)
		}
	}
	return nil
}

func TestDecoding(t *testing.T) {
	s := inmem.New()

	erasureBuffer := make([][]byte, 128)
	dataShardCount := 100
	ctx := context.TODO()

	sAddresses, pAddresses, err := initData(ctx, erasureBuffer, dataShardCount, s)
	if err != nil {
		t.Fatal(err)
	}

	getter := getter.New(sAddresses, pAddresses, s, s)
	// sanity check - all data shards are retrievable
	for i := 0; i < dataShardCount; i++ {
		ch, err := getter.Get(ctx, sAddresses[i])
		if err != nil {
			t.Fatalf("address %s at index %d is not retrievable by redundancy getter", sAddresses[i], i)
		}
		if !bytes.Equal(ch.Data(), erasureBuffer[i]) {
			t.Fatalf("retrieved chunk data differ from the original at index %d", i)
		}
	}

	// remove maximum possible chunks from storage
	removeChunkCount := len(erasureBuffer) - dataShardCount
	for i := 0; i < removeChunkCount; i++ {
		err := s.Delete(ctx, sAddresses[i])
		if err != nil {
			t.Fatal(err)
		}
	}

	err = dataShardsAvailable(ctx, sAddresses, s) // sanity check
	if err == nil {
		t.Fatalf("some data shards should be missing")
	}
	ch, err := getter.Get(ctx, sAddresses[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ch.Data(), erasureBuffer[0]) {
		t.Fatalf("retrieved chunk data differ from the original at index %d", 0)
	}
	err = dataShardsAvailable(ctx, sAddresses, s)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryLimits(t *testing.T) {
	s := inmem.New()

	erasureBuffer := make([][]byte, 8)
	dataShardCount := 5
	ctx := context.TODO()

	sAddresses, pAddresses, err := initData(ctx, erasureBuffer, dataShardCount, s)
	if err != nil {
		t.Fatal(err)
	}

	_getter := getter.New(sAddresses, pAddresses, s, s)

	// remove more chunks that can be corrected by erasure code
	removeChunkCount := len(erasureBuffer) - dataShardCount + 1
	for i := 0; i < removeChunkCount; i++ {
		err := s.Delete(ctx, sAddresses[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = _getter.Get(ctx, sAddresses[0])
	if !getter.IsCannotRecoverError(err, 1) {
		t.Fatal(err)
	}

	// call once more
	_, err = _getter.Get(ctx, sAddresses[0])
	if !getter.IsNotRecoveredError(err, sAddresses[0].String()) {
		t.Fatal(err)
	}
}

func TestNoRedundancyOnRecovery(t *testing.T) {
	s := inmem.New()

	dataShardCount := 5
	erasureBuffer := make([][]byte, dataShardCount)
	ctx := context.TODO()

	sAddresses, pAddresses, err := initData(ctx, erasureBuffer, dataShardCount, s)
	if err != nil {
		t.Fatal(err)
	}

	_getter := getter.New(sAddresses, pAddresses, s, s)

	// remove one chunk that trying to request later
	err = s.Delete(ctx, sAddresses[0])
	if err != nil {
		t.Fatal(err)
	}
	_, err = _getter.Get(ctx, sAddresses[0])
	if !getter.IsNoRedundancyError(err, sAddresses[0].String()) {
		t.Fatal(err)
	}
}

func TestNoDataAddressIncluded(t *testing.T) {
	s := inmem.New()

	erasureBuffer := make([][]byte, 8)
	dataShardCount := 5
	ctx := context.TODO()

	sAddresses, pAddresses, err := initData(ctx, erasureBuffer, dataShardCount, s)
	if err != nil {
		t.Fatal(err)
	}

	_getter := getter.New(sAddresses, pAddresses, s, s)

	// trying to retrieve a parity address
	_, err = _getter.Get(ctx, pAddresses[0])
	if !getter.IsNoDataAddressIncludedError(err, pAddresses[0].String()) {
		t.Fatal(err)
	}
}

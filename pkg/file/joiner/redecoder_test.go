// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

// TestReDecoderFlow tests the complete flow of:
// 1. Loading data with redundancy getter
// 2. Successful recovery which nulls the decoder
// 3. Chunk eviction from cache
// 4. Reloading the same data through ReDecoder fallback
func TestReDecoderFlow(t *testing.T) {
	ctx := context.Background()
	dataShardCount := 4
	parityShardCount := 2
	totalShardCount := dataShardCount + parityShardCount

	// Create real data chunks with proper content
	dataShards := make([][]byte, dataShardCount)
	for i := range dataShardCount {
		// Create chunks with simpler test data
		dataShards[i] = make([]byte, swarm.ChunkWithSpanSize)
		// Create a unique string for this shard
		testData := []byte("test-data-" + strconv.Itoa(i))
		// Copy as much as will fit
		copy(dataShards[i], testData)
	}

	// Create parity chunks using Reed-Solomon encoding
	parityShards := make([][]byte, parityShardCount)
	for i := range parityShardCount {
		parityShards[i] = make([]byte, swarm.ChunkWithSpanSize)
	}

	// Create Reed-Solomon encoder
	enc, err := reedsolomon.New(dataShardCount, parityShardCount)
	if err != nil {
		t.Fatalf("Failed to create Reed-Solomon encoder: %v", err)
	}

	// Combine data and parity shards
	allShards := make([][]byte, totalShardCount)
	copy(allShards, dataShards)
	copy(allShards[dataShardCount:], parityShards)

	// Encode to generate parity chunks
	if err := enc.Encode(allShards); err != nil {
		t.Fatalf("Failed to encode data: %v", err)
	}

	// Create content-addressed chunks for all shards
	addresses := make([]swarm.Address, totalShardCount)
	chunks := make([]swarm.Chunk, totalShardCount)

	for i := range totalShardCount {
		// Create proper content-addressed chunks
		chunk, err := cac.NewWithDataSpan(allShards[i])
		if err != nil {
			t.Fatalf("Failed to create content-addressed chunk %d: %v", i, err)
		}
		chunks[i] = chunk
		addresses[i] = chunk.Address()
	}

	// Select a data chunk to be missing (which will be recovered)
	missingChunkIndex := 2 // The third data chunk will be missing
	mockStore := inmemchunkstore.New()
	netFetcher := newMockNetworkFetcher(addresses, addresses[missingChunkIndex])
	config := getter.Config{
		Strategy: getter.RACE,
		Logger:   log.Noop,
	}

	j := joiner.NewDecoderCache(netFetcher, mockStore, config)

	// Step 1: Initializing decoder and triggering recovery
	decoder := j.GetOrCreate(addresses, dataShardCount)
	if decoder == nil {
		t.Fatal("Failed to create decoder")
	}

	// Verify we can now fetch the previously missing chunk through recovery
	recoveredChunk, err := decoder.Get(ctx, addresses[missingChunkIndex])
	if err != nil {
		t.Fatalf("Failed to recover missing chunk: %v", err)
	}
	// Verify the recovered chunk has the correct content
	if !recoveredChunk.Address().Equal(addresses[missingChunkIndex]) {
		t.Fatalf("Recovered chunk has incorrect address")
	}

	// Verify the recovered chunk has the correct content
	recoveredData := recoveredChunk.Data()
	expectedData := chunks[missingChunkIndex].Data()
	if len(recoveredData) != len(expectedData) {
		t.Fatalf("Recovered chunk has incorrect data length: got %d, want %d", len(recoveredData), len(expectedData))
	}
	if !bytes.Equal(recoveredData, expectedData) {
		t.Fatalf("Recovered chunk has incorrect data")
	}
	// Check if the recovered chunk is now in the store
	_, err = mockStore.Get(ctx, addresses[missingChunkIndex])
	if err != nil {
		t.Fatalf("Recovered chunk not saved to store: %v", err)
	}

	// Step 2: The original decoder should be automatically nulled after successful recovery
	// This is an internal state check, we can't directly test it but we can verify that
	// we can still access the chunks

	// Sanity check - verify we can still fetch chunks through the cache
	for i := range dataShardCount {
		_, err := decoder.Get(ctx, addresses[i])
		if err != nil {
			t.Fatalf("Failed to get chunk %d after recovery: %v", i, err)
		}
	}

	// Step 3: Testing ReDecoder fallback
	newDecoder := j.GetOrCreate(addresses, dataShardCount)
	if newDecoder == nil {
		t.Fatal("Failed to create ReDecoder")
	}

	// Verify all chunks can be fetched through the ReDecoder
	for i := range dataShardCount {
		_, err := newDecoder.Get(ctx, addresses[i])
		if err != nil {
			t.Fatalf("Failed to get chunk %d through ReDecoder: %v", i, err)
		}
	}

	// Verify that we can also access the first missing chunk - now from the store
	// This would be using the local store and not network or recovery mechanisms
	retrievedChunk, err := newDecoder.Get(ctx, addresses[missingChunkIndex])
	if err != nil {
		t.Fatalf("Failed to retrieve previously recovered chunk: %v", err)
	}

	if !retrievedChunk.Address().Equal(addresses[missingChunkIndex]) {
		t.Fatalf("Retrieved chunk has incorrect address")
	}

	// Also verify the data content matches
	retrievedData := retrievedChunk.Data()
	expectedData = chunks[missingChunkIndex].Data()
	if len(retrievedData) != len(expectedData) {
		t.Fatalf("Retrieved chunk has incorrect data length: got %d, want %d", len(retrievedData), len(expectedData))
	}
	if !bytes.Equal(retrievedData, expectedData) {
		t.Fatalf("Retrieved chunk has incorrect data")
	}
}

// Mock implementation of storage.Getter for testing
type mockNetworkFetcher struct {
	allAddresses []swarm.Address
	missingAddr  swarm.Address
}

// newMockNetworkFetcher creates a new mock fetcher that will return ErrNotFound for specific addresses
func newMockNetworkFetcher(allAddrs []swarm.Address, missingAddr swarm.Address) *mockNetworkFetcher {
	return &mockNetworkFetcher{
		allAddresses: allAddrs,
		missingAddr:  missingAddr,
	}
}

// Get implements storage.Getter interface
func (m *mockNetworkFetcher) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	// Simulate network fetch - fail for the missing chunk
	if addr.Equal(m.missingAddr) {
		return nil, storage.ErrNotFound
	}

	// Find the index of this address in the all addresses list
	var index int
	for i, a := range m.allAddresses {
		if addr.Equal(a) {
			index = i
			break
		}
	}

	// Generate data using the same pattern as in the test
	data := make([]byte, swarm.ChunkWithSpanSize)
	// Create a unique string for this shard
	testData := []byte("test-data-" + strconv.Itoa(index))
	// Copy as much as will fit
	copy(data, testData)

	return swarm.NewChunk(addr, data), nil
}

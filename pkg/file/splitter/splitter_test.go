// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package splitter_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	mockbytes "gitlab.com/nolash/go-mockbytes"
)

// TestSplitIncomplete tests that the Split method returns an error if
// the amounts of bytes written does not match the data length passed to the method.
func TestSplitIncomplete(t *testing.T) {
	testData := make([]byte, 42)
	store := mock.NewStorer()
	s := splitter.NewSimpleSplitter(store)

	testDataReader := file.NewSimpleReadCloser(testData)
	_, err := s.Split(context.Background(), testDataReader, 41)
	if err == nil {
		t.Fatalf("expected error on EOF before full length write")
	}
}

// TestSplitSingleChunk hashes one single chunk and verifies
// that that corresponding chunk exist in the store afterwards.
func TestSplitSingleChunk(t *testing.T) {
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	testData, err := g.SequentialBytes(swarm.ChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	store := mock.NewStorer()
	s := splitter.NewSimpleSplitter(store)

	testDataReader := file.NewSimpleReadCloser(testData)
	resultAddress, err := s.Split(context.Background(), testDataReader, int64(len(testData)))
	if err != nil {
		t.Fatal(err)
	}

	testHashHex := "c10090961e7682a10890c334d759a28426647141213abda93b096b892824d2ef"
	testHashAddress := swarm.MustParseHexAddress(testHashHex)
	if !testHashAddress.Equal(resultAddress) {
		t.Fatalf("expected %v, got %v", testHashAddress, resultAddress)
	}

	_, err = store.Get(context.Background(), storage.ModeGetRequest, resultAddress)
	if err != nil {
		t.Fatal(err)
	}
}

// TestSplitThreeLevels hashes enough data chunks in order to
// create a full chunk of intermediate hashes.
// It verifies that all created chunks exist in the store afterwards.
func TestSplitThreeLevels(t *testing.T) {
	// edge case selected from internal/job_test.go
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	testData, err := g.SequentialBytes(swarm.ChunkSize * swarm.Branches)
	if err != nil {
		t.Fatal(err)
	}

	store := mock.NewStorer()
	s := splitter.NewSimpleSplitter(store)

	testDataReader := file.NewSimpleReadCloser(testData)
	resultAddress, err := s.Split(context.Background(), testDataReader, int64(len(testData)))
	if err != nil {
		t.Fatal(err)
	}

	testHashHex := "3047d841077898c26bbe6be652a2ec590a5d9bd7cd45d290ea42511b48753c09"
	testHashAddress := swarm.MustParseHexAddress(testHashHex)
	if !testHashAddress.Equal(resultAddress) {
		t.Fatalf("expected %v, got %v", testHashAddress, resultAddress)
	}

	_, err = store.Get(context.Background(), storage.ModeGetRequest, resultAddress)
	if err != nil {
		t.Fatal(err)
	}

	rootChunk, err := store.Get(context.Background(), storage.ModeGetRequest, resultAddress)
	if err != nil {
		t.Fatal(err)
	}

	rootData := rootChunk.Data()[8:]
	for i := 0; i < swarm.ChunkSize; i += swarm.SectionSize {
		dataAddressBytes := rootData[i : i+swarm.SectionSize]
		dataAddress := swarm.NewAddress(dataAddressBytes)
		_, err := store.Get(context.Background(), storage.ModeGetRequest, dataAddress)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestUnalignedSplit tests that correct hash is generated regarless of
// individual write sizes at the source of the data.
func TestUnalignedSplit(t *testing.T) {
	var (
		storer    storage.Storer = mock.NewStorer()
		chunkPipe                = file.NewChunkPipe()
	)

	// test vector taken from pkg/file/testing/vector.go
	var (
		dataLen       int64 = swarm.ChunkSize*2 + 32
		expectAddrHex       = "61416726988f77b874435bdd89a419edc3861111884fd60e8adf54e2f299efd6"
		g                   = mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	)

	// generate test vector data content
	content, err := g.SequentialBytes(int(dataLen))
	if err != nil {
		t.Fatal(err)
	}

	// perform the split in a separate thread
	sp := splitter.NewSimpleSplitter(storer)
	ctx := context.Background()
	doneC := make(chan swarm.Address)
	errC := make(chan error)
	go func() {
		addr, err := sp.Split(ctx, chunkPipe, dataLen)
		if err != nil {
			errC <- err
		} else {
			doneC <- addr
		}
		close(doneC)
		close(errC)
	}()

	// perform the writes in unaligned bursts
	writeSizes := []int{swarm.ChunkSize - 40, 40 + 32, swarm.ChunkSize}
	contentBuf := bytes.NewReader(content)
	cursor := 0
	for _, writeSize := range writeSizes {
		data := make([]byte, writeSize)
		_, err = contentBuf.Read(data)
		if err != nil {
			t.Fatal(err)
		}
		c, err := chunkPipe.Write(data)
		if err != nil {
			t.Fatal(err)
		}
		cursor += c
	}
	err = chunkPipe.Close()
	if err != nil {
		t.Fatal(err)
	}

	// read and hopefully not weep
	timer := time.NewTimer(time.Millisecond * 100)
	select {
	case addr := <-doneC:
		expectAddr := swarm.MustParseHexAddress(expectAddrHex)
		if !expectAddr.Equal(addr) {
			t.Fatalf("addr mismatch, expected %s, got %s", expectAddr, addr)
		}
	case err := <-errC:
		t.Fatal(err)
	case <-timer.C:
		t.Fatal("timeout")
	}

}

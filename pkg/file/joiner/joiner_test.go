// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestJoiner verifies that a newly created joiner
// returns the data stored in the store for a given reference
func TestJoiner(t *testing.T) {
	store := mock.NewStorer()
	joiner := joiner.NewSimpleJoiner(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	_, _, err = joiner.Join(ctx, swarm.ZeroAddress)
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound for %x", swarm.ZeroAddress)
	}

	mockAddrHex := fmt.Sprintf("%064s", "2a")
	mockAddr := swarm.MustParseHexAddress(mockAddrHex)
	mockData := []byte("foo")
	mockChunk := swarm.NewChunk(mockAddr, mockData)
	_, err = store.Put(ctx, storage.ModePutRequest, mockChunk)
	if err != nil {
		t.Fatal(err)
	}

	joinReader, l, err := joiner.Join(ctx, mockAddr)
	if l != int64(len(mockData)) {
		t.Fatalf("expected join data length %d, got %d", len(mockData), l)
	}
	joinData, err := ioutil.ReadAll(joinReader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(joinData, mockData) {
		t.Fatalf("retrieved data '%x' not like original data '%x'", joinData, mockData)
	}
}

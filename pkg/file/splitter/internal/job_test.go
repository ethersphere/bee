// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/file/splitter/internal"
	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	start = 0
	end   = test.GetVectorCount()
)

// TestSplitterJobPartialSingleChunk passes sub-chunk length data to the splitter,
// verifies the correct hash is returned, and that write after Sum/complete Write
// returns error.
func TestSplitterJobPartialSingleChunk(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := []byte("foo")
	j := internal.NewSimpleSplitterJob(ctx, store, int64(len(data)))

	c, err := j.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if c < len(data) {
		t.Fatalf("short write %d", c)
	}

	hashResult := j.Sum(nil)
	addressResult := swarm.NewAddress(hashResult)

	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)
	if !addressResult.Equal(address) {
		t.Fatalf("expected %v, got %v", address, addressResult)
	}

	_, err = j.Write([]byte("bar"))
	if err == nil {
		t.Fatal("expected error writing after write/sum complete")
	}
}

// TestSplitterJobVector verifies file hasher results of legacy test vectors
func TestSplitterJobVector(t *testing.T) {
	for i := start; i < end; i++ {
		dataLengthStr := strconv.Itoa(i)
		t.Run(dataLengthStr, testSplitterJobVector)
	}
}

func testSplitterJobVector(t *testing.T) {
	var (
		paramstring = strings.Split(t.Name(), "/")
		dataIdx, _  = strconv.ParseInt(paramstring[1], 10, 0)
		store       = mock.NewStorer()
	)

	data, expect := test.GetVector(t, int(dataIdx))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := internal.NewSimpleSplitterJob(ctx, store, int64(len(data)))

	for i := 0; i < len(data); i += swarm.ChunkSize {
		l := swarm.ChunkSize
		if len(data)-i < swarm.ChunkSize {
			l = len(data) - i
		}
		c, err := j.Write(data[i : i+l])
		if err != nil {
			t.Fatal(err)
		}
		if c < l {
			t.Fatalf("short write %d", c)
		}
	}

	actualBytes := j.Sum(nil)
	actual := swarm.NewAddress(actualBytes)

	if !expect.Equal(actual) {
		t.Fatalf("expected %v, got %v", expect, actual)
	}
}

// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/cmd/bee/cmd"
	"github.com/ethersphere/bee/pkg/node"
	"github.com/ethersphere/bee/pkg/postage"
	storagetest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	kademlia "github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

func TestDBExportImport(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	export := t.TempDir() + "/export.tar"

	ctx := context.Background()
	db1 := newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          testutil.NewLogger(t),
		ReserveCapacity: node.ReserveCapacity,
	}, dir1)

	chunks := make(map[string]int)
	nChunks := 10
	for i := 0; i < nChunks; i++ {
		ch := storagetest.GenerateTestRandomChunk()
		err := db1.ReservePut(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("put chunk: ", ch.Address().String())
		chunks[ch.Address().String()] = 0
	}
	db1.Close()

	err := newCommand(t, cmd.WithArgs("db", "export", "reserve", export, "--data-dir", dir1)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	err = newCommand(t, cmd.WithArgs("db", "import", "reserve", export, "--data-dir", dir2)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	db2 := newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          testutil.NewLogger(t),
		ReserveCapacity: node.ReserveCapacity,
	}, dir2)

	err = db2.ReserveIterateChunks(func(chunk swarm.Chunk) (bool, error) {
		chunks[chunk.Address().String()]++
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	db2.Close()

	for k, v := range chunks {
		if v != 1 {
			t.Errorf("chunk %s missing", k)
		}
	}
}

func TestDBExportImportPinning(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	export := t.TempDir() + "/export.tar"

	ctx := context.Background()
	db1 := newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          testutil.NewLogger(t),
		ReserveCapacity: node.ReserveCapacity,
	}, dir1)

	chunks := make(map[string]int)
	pins := make(map[string]any)
	nChunks := 10

	for i := 0; i < 2; i++ {
		rootAddr := swarm.RandAddress(t)
		collection, err := db1.NewCollection(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for j := 0; j < nChunks; j++ {
			ch := storagetest.GenerateTestRandomChunk()
			err = collection.Put(ctx, ch)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println("collection ", rootAddr.String(), " put chunk: ", ch.Address().String())
			chunks[ch.Address().String()] = 0
		}
		err = collection.Done(rootAddr)
		if err != nil {
			t.Fatal(err)
		}
		pins[rootAddr.String()] = nil
	}

	addresses, err := db1.Pins()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addresses {
		fmt.Println("pin: ", addr.String())
	}
	db1.Close()

	err = newCommand(t, cmd.WithArgs("db", "export", "pinning", export, "--data-dir", dir1)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	err = newCommand(t, cmd.WithArgs("db", "import", "pinning", export, "--data-dir", dir2)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	db2 := newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          testutil.NewLogger(t),
		ReserveCapacity: node.ReserveCapacity,
	}, dir2)
	addresses, err = db2.Pins()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addresses {
		if _, ok := pins[addr.String()]; !ok {
			t.Errorf("pin %s missing", addr.String())
		}
	}

	for _, addr := range addresses {
		rootAddr, err := swarm.ParseHexAddress(addr.String())
		if err != nil {
			t.Fatal(err)
		}
		err = db2.IteratePinCollection(rootAddr, func(ch swarm.Address) (bool, error) {
			chunks[ch.String()]++
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	db2.Close()

	for k, v := range chunks {
		if v != 1 {
			t.Errorf("chunk %s missing", k)
		}
	}
}

func TestMarshalChunk(t *testing.T) {
	t.Parallel()
	ch := storagetest.GenerateTestRandomChunk()
	b, err := cmd.MarshalChunkToBinary(ch)
	if err != nil {
		t.Fatal(err)
	}
	want := 4 + len(ch.Data()) + postage.StampSize
	if len(b) != want {
		t.Fatalf("got %d, want %d", len(b), want)
	}

	ch1, err := cmd.UnmarshalChunkFromBinary(b, ch.Address().String())
	if err != nil {
		t.Fatal(err)
	}
	if !ch1.Address().Equal(ch.Address()) {
		t.Fatalf("address mismatch: got %s, want %s", ch1.Address(), ch.Address())
	}
	if !bytes.Equal(ch1.Data(), ch.Data()) {
		t.Fatalf("data mismatch: got %v, want %v", ch1.Data(), ch.Data())
	}
}

func newTestDB(t *testing.T, ctx context.Context, opts *storer.Options, dir string) *storer.DB {
	t.Helper()
	db, err := storer.New(ctx, dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

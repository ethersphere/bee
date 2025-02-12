// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/cmd/bee/cmd"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	storagetest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
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
		ReserveCapacity: storer.DefaultReserveCapacity,
	}, dir1)

	chunks := make(map[string]int)
	nChunks := 10
	for i := 0; i < nChunks; i++ {
		ch := storagetest.GenerateTestRandomChunk()
		err := db1.ReservePutter().Put(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
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
		ReserveCapacity: storer.DefaultReserveCapacity,
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
		ReserveCapacity: storer.DefaultReserveCapacity,
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
			chunks[ch.Address().String()] = 0
		}
		err = collection.Done(rootAddr)
		if err != nil {
			t.Fatal(err)
		}
		pins[rootAddr.String()] = nil
	}

	db1.Close()

	err := newCommand(t, cmd.WithArgs("db", "export", "pinning", export, "--data-dir", dir1)).Execute()
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
		ReserveCapacity: storer.DefaultReserveCapacity,
	}, dir2)
	addresses, err := db2.Pins()
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

// TestDBNuke_FLAKY is flaky on windows.
func TestDBNuke_FLAKY(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	ctx := context.Background()
	db := newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          log.Noop,
		ReserveCapacity: storer.DefaultReserveCapacity,
	}, dataDir)

	nChunks := 10
	for i := 0; i < nChunks; i++ {
		ch := storagetest.GenerateTestRandomChunk()
		err := db.ReservePutter().Put(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
	}
	info, err := db.DebugInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Reserve.TotalSize != nChunks {
		t.Errorf("got reserve size before nuke: %d, want %d", info.Reserve.TotalSize, nChunks)
	}

	db.Close()

	err = newCommand(t, cmd.WithArgs("db", "nuke", "--data-dir", dataDir)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	db = newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          log.Noop,
		ReserveCapacity: storer.DefaultReserveCapacity,
	}, path.Join(dataDir, "localstore"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	info, err = db.DebugInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Reserve.TotalSize != 0 {
		t.Errorf("got reserve size after nuke: %d, want %d", info.Reserve.TotalSize, 0)
	}
}

func TestDBInfo(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	ctx := context.Background()
	db1 := newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          testutil.NewLogger(t),
		ReserveCapacity: storer.DefaultReserveCapacity,
	}, dir1)

	nChunks := 10
	for i := 0; i < nChunks; i++ {
		ch := storagetest.GenerateTestRandomChunk()
		err := db1.ReservePutter().Put(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
	}
	info, err := db1.DebugInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Reserve.TotalSize != nChunks {
		t.Errorf("got reserve size before nuke: %d, want %d", info.Reserve.TotalSize, nChunks)
	}

	db1.Close()

	var buf bytes.Buffer
	err = newCommand(t, cmd.WithArgs("db", "info", "--data-dir", dir1), cmd.WithOutput(&buf)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), fmt.Sprintf("\"msg\"=\"reserve\" \"size_within_radius\"=%d \"total_size\"=%d \"capacity\"=%d", nChunks, nChunks, storer.DefaultReserveCapacity)) {
		t.Fatal("reserve info not correct")
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

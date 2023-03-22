// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/exp/slices"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/leveldbstore"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
)

func TestTxChunkStore_Recovery(t *testing.T) {
	t.Parallel()

	store, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}

	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	txChunkStore := chunkstore.NewTxChunkStore(leveldbstore.NewTxStore(store), sharky)
	t.Cleanup(func() {
		if err := txChunkStore.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	chunks := chunktest.GenerateTestRandomChunks(10)
	lessFn := func(i, j swarm.Chunk) int { return i.Address().Compare(j.Address()) }
	slices.SortFunc(chunks, lessFn)

	// Sore half of the chunks within a transaction and commit it.
	tx := txChunkStore.NewTx(storage.NewTxState(context.TODO()))
	for i := 0; i < len(chunks)/2; i++ {
		if err = tx.Put(context.TODO(), chunks[i]); err != nil {
			t.Fatalf("put chunk: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Delete the first stored half of the chunks and store
	// the other half and don't commit or revert the transaction.
	tx = txChunkStore.NewTx(storage.NewTxState(context.TODO()))
	for i := 0; i < len(chunks)/2; i++ {
		if err = tx.Delete(context.TODO(), chunks[i].Address()); err != nil {
			t.Fatalf("put chunk: %v", err)
		}
	}
	for i := len(chunks) / 2; i < len(chunks); i++ {
		if err = tx.Put(context.TODO(), chunks[i]); err != nil {
			t.Fatalf("put chunk: %v", err)
		}
	}
	// Do not commit or rollback the transaction as
	// if the process crashes and attempt to recover.
	if err := txChunkStore.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}

	// Check that the store is in the state we expect.
	var (
		have []swarm.Chunk
		want = chunks[:len(chunks)/2]
	)
	if err := txChunkStore.Iterate(
		context.TODO(),
		func(chunk swarm.Chunk) (stop bool, err error) {
			have = append(have, chunk)
			return false, nil
		},
	); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	lessFn2 := func(i, j swarm.Chunk) bool { return i.Address().Compare(j.Address()) < 0 }
	if diff := cmp.Diff(want, have, cmpopts.SortSlices(lessFn2)); diff != "" {
		t.Fatalf("recovered store data mismatch (-want +have):\n%s", diff)
	}
}

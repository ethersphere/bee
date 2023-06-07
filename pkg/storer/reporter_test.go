// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	storage "github.com/ethersphere/bee/pkg/storage"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

func testReporter(t *testing.T, newStorer func() (*storer.DB, error)) {
	t.Helper()

	chunks := chunktesting.GenerateTestRandomChunks(3)

	lstore, err := newStorer()
	if err != nil {
		t.Fatal(err)
	}

	session, err := lstore.NewSession()
	if err != nil {
		t.Fatal(err)
	}

	putter, err := lstore.Upload(context.Background(), false, session.TagID)
	if err != nil {
		t.Fatal(err)
	}

	for _, ch := range chunks {
		err = putter.Put(context.Background(), ch)
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("report", func(t *testing.T) {
		t.Run("commit", func(t *testing.T) {
			err := lstore.Report(context.Background(), chunks[0], storage.ChunkSynced)
			if err != nil {
				t.Fatalf("Report(...): unexpected error %v", err)
			}

			wantTI := storer.SessionInfo{
				TagID:     session.TagID,
				Split:     0,
				Seen:      0,
				Sent:      0,
				Synced:    1,
				Stored:    0,
				StartedAt: session.StartedAt,
			}

			gotTI, err := lstore.Session(session.TagID)
			if err != nil {
				t.Fatalf("Session(...): unexpected error: %v", err)
			}

			if diff := cmp.Diff(wantTI, gotTI); diff != "" {
				t.Fatalf("unexpected tag item (-want +have):\n%s", diff)
			}

			has, err := lstore.Repo().ChunkStore().Has(context.Background(), chunks[0].Address())
			if err != nil {
				t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
			}
			if has {
				t.Fatalf("expected chunk %s to not be found", chunks[0].Address())
			}
		})

		t.Run("rollback", func(t *testing.T) {
			want := errors.New("dummy error")
			lstore.SetRepoStorePutHook(func(item storage.Item) error {
				if item.Namespace() == "tagItem" {
					return want
				}
				return nil
			})
			have := lstore.Report(context.Background(), chunks[1], storage.ChunkSynced)
			if !errors.Is(have, want) {
				t.Fatalf("unexpected error on Report: want %v have %v", want, have)
			}

			wantTI := storer.SessionInfo{
				TagID:     session.TagID,
				Split:     0,
				Seen:      0,
				Sent:      0,
				Synced:    1,
				Stored:    0,
				StartedAt: session.StartedAt,
			}

			gotTI, err := lstore.Session(session.TagID)
			if err != nil {
				t.Fatalf("Session(...): unexpected error: %v", err)
			}

			if diff := cmp.Diff(wantTI, gotTI); diff != "" {
				t.Fatalf("unexpected tag item (-want +have):\n%s", diff)
			}

			has, err := lstore.Repo().ChunkStore().Has(context.Background(), chunks[1].Address())
			if err != nil {
				t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
			}
			if !has {
				t.Fatalf("expected chunk %s to be found", chunks[1].Address())
			}
		})
	})
}

func TestReporter(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testReporter(t, func() (*storer.DB, error) {

			opts := dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)

			return storer.New(context.Background(), "", opts)
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		opts := dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)

		testReporter(t, diskStorer(t, opts))
	})
}

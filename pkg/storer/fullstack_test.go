// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"testing"
	"time"

	pullerMock "github.com/ethersphere/bee/v2/pkg/puller/mock"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestFullStackPut spins up a disk-backed storer and writes a valid CAC and
// a valid SOC into each of the cache, upload, pin and reserve stores, and checks
// that invalid CACs and SOCs are rejected.
func TestFullStackPut(t *testing.T) {
	t.Parallel()

	stores := map[string]func(t *testing.T, ctx context.Context, st *storer.DB, ch swarm.Chunk) error{
		"cache": func(t *testing.T, ctx context.Context, st *storer.DB, ch swarm.Chunk) error {
			t.Helper()
			return st.Cache().Put(ctx, ch)
		},
		"upload": func(t *testing.T, ctx context.Context, st *storer.DB, ch swarm.Chunk) error {
			t.Helper()
			tag, err := st.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}
			session, err := st.Upload(ctx, false, tag.TagID)
			if err != nil {
				t.Fatalf("Upload(...): unexpected error: %v", err)
			}
			if err := session.Put(ctx, ch); err != nil {
				_ = session.Cleanup()
				return err
			}
			return session.Done(ch.Address())
		},
		"pin": func(t *testing.T, ctx context.Context, st *storer.DB, ch swarm.Chunk) error {
			t.Helper()
			session, err := st.NewCollection(ctx)
			if err != nil {
				t.Fatalf("NewCollection(...): unexpected error: %v", err)
			}
			if err := session.Put(ctx, ch); err != nil {
				_ = session.Cleanup()
				return err
			}
			return session.Done(ch.Address())
		},
		"reserve": func(t *testing.T, ctx context.Context, st *storer.DB, ch swarm.Chunk) error {
			t.Helper()
			return st.ReservePutter().Put(ctx, ch)
		},
	}

	newStorer := func(t *testing.T, ctx context.Context, storeName string) *storer.DB {
		t.Helper()
		opts := dbTestOps(swarm.RandAddress(t), 1000, nil, nil, time.Minute)
		st := makeDiskStorer(t, opts)
		if storeName == "reserve" {
			readyC := make(chan struct{})
			st.StartReserveWorker(ctx, pullerMock.NewMockRateReporter(0), networkRadiusFunc(0), readyC)
			<-readyC
		}
		return st
	}

	for storeName, put := range stores {
		for _, tc := range chunk.ChunkValidityCases(t) {
			t.Run(storeName+"/"+tc.Name, func(t *testing.T) {
				t.Parallel()

				ctx := context.Background()
				st := newStorer(t, ctx, storeName)
				ch := tc.Chunk

				err := put(t, ctx, st, ch)
				if !tc.Valid {
					if err == nil {
						t.Fatalf("put %s into %s: expected error, got nil", tc.Name, storeName)
					}
					has, err := st.ChunkStore().Has(ctx, ch.Address())
					if err != nil {
						t.Fatalf("ChunkStore().Has(...): unexpected error: %v", err)
					}
					if has {
						t.Fatalf("invalid chunk %s was stored in %s", ch.Address(), storeName)
					}
					return
				}
				if err != nil {
					t.Fatalf("put %s into %s: unexpected error: %v", tc.Name, storeName, err)
				}

				got, err := st.Lookup().Get(ctx, ch.Address())
				if err != nil {
					t.Fatalf("Lookup().Get(...): unexpected error: %v", err)
				}
				if !got.Equal(ch) {
					t.Fatalf("chunk mismatch: want %s, got %s", ch.Address(), got.Address())
				}
			})
		}
	}
}

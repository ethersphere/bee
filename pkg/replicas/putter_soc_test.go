// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was created as a copy of the original putter_test.go file
// and tailored to the socPutter implementation.

package replicas_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type putterSession struct {
	chunkStore storage.ChunkStore
	getErrors  func(context.Context, swarm.Address) error
	putErrors  func(context.Context, swarm.Address) error
}

func (tbp *putterSession) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {

	g := tbp.getErrors
	if g != nil {
		return nil, g(ctx, addr)
	}
	return tbp.chunkStore.Get(ctx, addr)
}

func (p *putterSession) Put(ctx context.Context, ch swarm.Chunk) error {
	g := p.putErrors
	if g != nil {
		return g(ctx, ch.Address())
	}

	return p.chunkStore.Put(ctx, ch)
}

func (p *putterSession) Done(address swarm.Address) error { return nil }

func (p *putterSession) Cleanup() error { return nil }

func TestSocPutter(t *testing.T) {
	t.Parallel()

	// test key to sign soc chunks
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	tcs := []struct {
		level  redundancy.Level
		length int
	}{
		{0, 1},
		{1, 1},
		{2, 1},
		{3, 1},
		{4, 1},
		{0, 4096},
		{1, 4096},
		{2, 4096},
		{3, 4096},
		{4, 4096},
	}
	for _, tc := range tcs {
		t.Run(fmt.Sprintf("redundancy:%d, size:%d", tc.level, tc.length), func(t *testing.T) {
			buf := make([]byte, tc.length)
			if _, err := io.ReadFull(rand.Reader, buf); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			ch, err := cac.New(buf)
			if err != nil {
				t.Fatal(err)
			}
			// create soc from cac
			id := make([]byte, swarm.HashSize)
			if _, err := rand.Read(id); err != nil {
				t.Fatal(err)
			}
			s := soc.New(id, ch)
			sch, err := s.Sign(signer)
			if err != nil {
				t.Fatal(err)
			}

			store := inmemchunkstore.New()
			defer store.Close()
			session := &putterSession{chunkStore: store}
			p := replicas.NewSocPutter(session, tc.level)

			if err := p.Put(ctx, sch); err != nil {
				t.Fatalf("expected no error. got %v", err)
			}
			var addrs []swarm.Address
			orig := false
			_ = store.Iterate(ctx, func(chunk swarm.Chunk) (stop bool, err error) {
				if sch.Address().Equal(chunk.Address()) {
					orig = true
					return false, nil
				}
				if !soc.Valid(chunk) {
					t.Fatalf("chunk %v is not a valid SOC chunk", chunk.Address())
				}
				addrs = append(addrs, chunk.Address())
				return false, nil
			})
			if !orig {
				t.Fatal("original chunk missing")
			}
			t.Run("dispersion", func(t *testing.T) {
				if err := dispersed(tc.level, addrs); err != nil {
					t.Fatalf("addresses are not dispersed: %v", err)
				}
			})
			t.Run("attempts", func(t *testing.T) {
				count := tc.level.GetReplicaCount()
				if len(addrs) != count {
					t.Fatalf("incorrect number of attempts. want %v, got %v", count, len(addrs))
				}
			})

			t.Run("replication", func(t *testing.T) {
				if err := replicated(store, ch, addrs); err != nil {
					t.Fatalf("chunks are not replicas: %v", err)
				}
			})
		})
	}
	t.Run("error handling", func(t *testing.T) {
		tcs := []struct {
			name   string
			level  redundancy.Level
			length int
			f      func(*putterSession) *putterSession
			err    []error
		}{
			{"put errors", 4, 4096, func(tbp *putterSession) *putterSession {
				var j int32
				i := &j
				atomic.StoreInt32(i, 0)
				tbp.putErrors = func(ctx context.Context, _ swarm.Address) error {
					j := atomic.AddInt32(i, 1)
					<-time.After(10 * time.Millisecond)
					if j == 6 {
						return errTestA
					}
					if j == 12 {
						return errTestB
					}
					return nil
				}
				return tbp
			}, []error{errTestA, errTestB}},
			{"put latencies", 4, 4096, func(tbp *putterSession) *putterSession {
				var j int32
				i := &j
				atomic.StoreInt32(i, 0)
				tbp.putErrors = func(ctx context.Context, _ swarm.Address) error {
					j := atomic.AddInt32(i, 1)
					if j == 6 {
						select {
						case <-time.After(100 * time.Millisecond):
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					if j == 12 {
						return errTestA
					}
					return nil
				}
				return tbp
			}, []error{errTestA, context.DeadlineExceeded}},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				buf := make([]byte, tc.length)
				if _, err := io.ReadFull(rand.Reader, buf); err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
				ch, err := cac.New(buf)
				if err != nil {
					t.Fatal(err)
				}

				id := make([]byte, swarm.HashSize)
				if _, err := rand.Read(id); err != nil {
					t.Fatal(err)
				}
				s := soc.New(id, ch)
				sch, err := s.Sign(signer)
				if err != nil {
					t.Fatal(err)
				}

				store := inmemchunkstore.New()
				defer store.Close()
				p := replicas.NewSocPutter(tc.f(&putterSession{chunkStore: store}), tc.level)
				errs := p.Put(ctx, sch)
				for _, err := range tc.err {
					if !errors.Is(errs, err) {
						t.Fatalf("incorrect error. want it to contain %v. got %v.", tc.err, errs)
					}
				}
			})
		}
	})

}

// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file is a copy of the original getter_test.go file
// and tailored to socGetter implementation.

package replicas_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

func TestSOCGetter(t *testing.T) {
	t.Parallel()
	// failure is a struct that defines a failure scenario to test
	type failure struct {
		name string
		err  error
		errf func(int, int) func(int) chan struct{}
	}
	// failures is a list of failure scenarios to test
	failures := []failure{
		{
			"timeout",
			context.Canceled,
			func(_, _ int) func(i int) chan struct{} {
				return func(i int) chan struct{} {
					return nil
				}
			},
		},
		{
			"not found",
			storage.ErrNotFound,
			func(_, _ int) func(i int) chan struct{} {
				c := make(chan struct{})
				close(c)
				return func(i int) chan struct{} {
					return c
				}
			},
		},
	}
	type test struct {
		name    string
		failure failure
		level   int
		count   int
		found   int
	}

	var tests []test
	for _, f := range failures {
		for level, c := range redundancy.GetReplicaCounts() {
			for j := 0; j <= c*2+1; j++ {
				tests = append(tests, test{
					name:    fmt.Sprintf("%s level %d count %d found %d", f.name, level, c, j),
					failure: f,
					level:   level,
					count:   c,
					found:   j,
				})
			}
		}
	}

	// initialise the base chunk
	chunkLen := 420
	buf := make([]byte, chunkLen)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		t.Fatal(err)
	}
	ch, err := cac.New(buf)
	if err != nil {
		t.Fatal(err)
	}
	// create soc from cac
	// test key to sign soc chunks
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	s := soc.New(id, ch)
	ch, err = s.Sign(signer)
	if err != nil {
		t.Fatal(err)
	}

	// reset retry interval to speed up tests
	retryInterval := replicas.SOCRetryInterval
	defer func() { replicas.SOCRetryInterval = retryInterval }()
	replicas.SOCRetryInterval = 100 * time.Millisecond

	// run the tests
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// initiate a chunk retrieval session using replicas.Getter
			// embedding a testGetter that simulates the behaviour of a chunk store
			store := newTestGetter(ch, tc.found, tc.failure.errf(tc.found, tc.count))
			g := replicas.NewSocGetter(store, redundancy.Level(tc.level))
			store.now = time.Now()
			ctx, cancel := context.WithCancel(context.Background())
			if tc.found > tc.count {
				wait := replicas.SOCRetryInterval / 2 * time.Duration(1+2*tc.level)
				go func() {
					time.Sleep(wait)
					cancel()
				}()
			}
			_, err := g.Get(ctx, ch.Address())
			replicas.Wait(g)
			cancel()

			// test the returned error
			if tc.found <= tc.count {
				if err != nil {
					t.Fatalf("expected no error. got %v", err)
				}
				// if j <= c, the original chunk should be retrieved and the context should be cancelled
				t.Run("retrievals cancelled", func(t *testing.T) {
					select {
					case <-time.After(100 * time.Millisecond):
						t.Fatal("timed out waiting for context to be cancelled")
					case <-store.cancelled:
					}
				})

			} else {
				if err == nil {
					t.Fatalf("expected error. got <nil>")
				}

				t.Run("returns correct error", func(t *testing.T) {
					if !errors.Is(err, replicas.ErrSwarmageddon) {
						t.Fatalf("incorrect error. want Swarmageddon. got %v", err)
					}
					if !errors.Is(err, tc.failure.err) {
						t.Fatalf("incorrect error. want it to wrap %v. got %v", tc.failure.err, err)
					}
				})
			}

			attempts := int(store.attempts.Load())
			// the original chunk should be among those attempted for retrieval
			addresses := store.addresses[:attempts]
			latencies := store.latencies[:attempts]
			t.Run("original address called", func(t *testing.T) {
				select {
				case <-time.After(100 * time.Millisecond):
					t.Fatal("timed out waiting form original address to be attempted for retrieval")
				case <-store.origCalled:
					i := store.origIndex
					if i > 2 {
						t.Fatalf("original address called too late. want at most 2 (preceding attempts). got %v (latency: %v)", i, latencies[i])
					}
					addresses = append(addresses[:i], addresses[i+1:]...)
					latencies = append(latencies[:i], latencies[i+1:]...)
					attempts--
				}
			})

			t.Run("retrieved count", func(t *testing.T) {
				if attempts > tc.count {
					t.Fatalf("too many attempts to retrieve a replica: want at most %v. got %v.", tc.count, attempts)
				}
				if tc.found > tc.count {
					if attempts < tc.count {
						t.Fatalf("too few attempts to retrieve a replica: want at least %v. got %v.", tc.count, attempts)
					}
					return
				}
				maxValue := 2
				for i := 1; i < tc.level && maxValue < tc.found; i++ {
					maxValue = maxValue * 2
				}
				if attempts > maxValue {
					t.Fatalf("too many attempts to retrieve a replica: want at most %v. got %v. latencies %v", maxValue, attempts, latencies)
				}
			})

			t.Run("dispersion", func(t *testing.T) {
				if err := dispersed(redundancy.Level(tc.level), ch, addresses); err != nil {
					t.Fatalf("addresses are not dispersed: %v", err)
				}
			})

			t.Run("latency", func(t *testing.T) {
				counts := redundancy.GetReplicaCounts()
				for i, latency := range latencies {
					multiplier := latency / replicas.SOCRetryInterval
					if multiplier > 0 && i < counts[multiplier-1] {
						t.Fatalf("incorrect latency for retrieving replica %d: %v", i, err)
					}
				}
			})
		})
	}
}

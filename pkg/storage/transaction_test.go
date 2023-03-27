// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
)

func TestTxState(t *testing.T) {
	t.Parallel()

	t.Run("lifecycle-normal", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		var (
			txs     = storage.NewTxState(ctx)
			timeout = 100 * time.Millisecond
		)

		if err := txs.IsDone(); err != nil {
			t.Fatalf("IsDone(): unexpected error: %v", err)
		}

		time.AfterFunc(timeout, func() {
			if err := txs.Done(); err != nil {
				t.Fatalf("Done(): unexpected error: %v", err)
			}
			if err := txs.Done(); !errors.Is(err, storage.ErrTxDone) {
				t.Fatalf("Done():\n\twant error: %v\n\thave error: %v", storage.ErrTxDone, err)
			}
		})

		func() {
			for timer := time.NewTimer(5 * timeout); ; {
				select {
				case <-txs.AwaitDone():
					if !timer.Stop() {
						<-timer.C
					}
					return
				case <-timer.C:
					select {
					case <-ctx.Done():
						t.Fatalf("parent context canceled")
					default:
						t.Fatalf("Done() did not release AwaitDone()")
					}
				}
			}
		}()

		if err := txs.IsDone(); !errors.Is(err, storage.ErrTxDone) {
			t.Fatalf("IsDone(): want error %v; have %v", storage.ErrTxDone, err)
		}

		select {
		case <-txs.AwaitDone():
		default:
			t.Error("AwaitDone() is blocking")
		}
	})

	t.Run("lifecycle-done-by-parent-ctx", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		var (
			txs     = storage.NewTxState(ctx)
			timeout = 100 * time.Millisecond
		)

		if err := txs.IsDone(); err != nil {
			t.Fatalf("IsDone(): unexpected error: %v", err)
		}

		time.AfterFunc(timeout, func() {
			cancel()
		})

		func() {
			for timer := time.NewTimer(5 * timeout); ; {
				select {
				case <-txs.AwaitDone():
					if !timer.Stop() {
						<-timer.C
					}
					return
				case <-timer.C:
					select {
					case <-ctx.Done():
						t.Fatalf("cancelation of parent context did not release AwaitDone()")
					default:
						t.Fatalf("parent context not canceled")
					}
				}
			}
		}()

		if err := txs.IsDone(); !errors.Is(err, context.Canceled) {
			t.Fatalf("IsDone():\n\twant error %v\n\thave %v", context.Canceled, err)
		}
		if err := txs.Done(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Done():\n\twant error: %v\n\thave error: %v", context.Canceled, err)
		}

		select {
		case <-txs.AwaitDone():
		default:
			t.Error("AwaitDone() is blocking")
		}
	})
}

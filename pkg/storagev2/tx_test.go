// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage_test

import (
	"context"
	"testing"
	"time"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

func TestTxState(t *testing.T) {
	t.Run("lifecycle-normal", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		var (
			txs     = storage.NewTxStatus(ctx)
			timeout = 100 * time.Millisecond
		)

		if have, want := txs.IsDone(), false; have != want {
			t.Fatalf("IsDone(): have %t; want %t", have, want)
		}

		time.AfterFunc(timeout, func() {
			txs.Done()
		})

	LOOP:
		for timer := time.NewTimer(2 * timeout); ; {
			select {
			case <-txs.AwaitDone():
				if !timer.Stop() {
					<-timer.C
				}
				break LOOP
			case <-timer.C:
				select {
				case <-ctx.Done():
					t.Fatalf("parent context canceled")
				default:
					t.Fatalf("Done() did not release AwaitDone()")
				}
			}
		}

		if have, want := txs.IsDone(), true; have != want {
			t.Errorf("IsDone(): have %t; want %t", have, want)
		}

		select {
		case <-txs.AwaitDone():
		default:
			t.Error("AwaitDone() is blocking")
		}
	})

	t.Run("lifecycle-done-by-parent-ctx", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		var (
			txs     = storage.NewTxStatus(ctx)
			timeout = 100 * time.Millisecond
		)

		if have, want := txs.IsDone(), false; have != want {
			t.Fatalf("IsDone(): have %t; want %t", have, want)
		}

		time.AfterFunc(timeout, func() {
			cancel()
		})

	LOOP:
		for timer := time.NewTimer(2 * timeout); ; {
			select {
			case <-txs.AwaitDone():
				if !timer.Stop() {
					<-timer.C
				}
				break LOOP
			case <-timer.C:
				select {
				case <-ctx.Done():
					t.Fatalf("cancelation of parent context did not release AwaitDone()")
				default:
					t.Fatalf("parent context not canceled")
				}
			}
		}

		if have, want := txs.IsDone(), true; have != want {
			t.Errorf("IsDone(): have %t; want %t", have, want)
		}

		select {
		case <-txs.AwaitDone():
		default:
			t.Error("AwaitDone() is blocking")
		}
	})
}

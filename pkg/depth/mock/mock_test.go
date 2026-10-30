// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/depth"
	"github.com/ethersphere/bee/v2/pkg/depth/mock"
)

var _ depth.Provider = (*mock.Provider)(nil)

func TestMock(t *testing.T) {
	t.Parallel()

	t.Run("wait blocks until set", func(t *testing.T) {
		t.Parallel()

		p := mock.New()
		if d, s := p.Depth(); d != 0 || s != depth.StateNoData {
			t.Fatalf("got (%d, %s)", d, s)
		}

		type result struct {
			d   uint8
			s   depth.State
			err error
		}
		res := make(chan result, 1)
		go func() {
			d, s, err := p.Wait(context.Background())
			res <- result{d, s, err}
		}()

		select {
		case <-res:
			t.Fatal("wait returned before set")
		case <-time.After(50 * time.Millisecond):
		}

		p.Set(0, depth.StateNoHistory)

		select {
		case r := <-res:
			if r.d != 0 || r.s != depth.StateNoHistory || r.err != nil {
				t.Fatalf("got (%d, %s, %v)", r.d, r.s, r.err)
			}
		case <-time.After(time.Second):
			t.Fatal("wait did not return after set")
		}
	})

	t.Run("wait returns context error", func(t *testing.T) {
		t.Parallel()

		p := mock.New()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, _, err := p.Wait(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("set known reaches subscriber", func(t *testing.T) {
		t.Parallel()

		p := mock.New()
		ch, unsub := p.Subscribe()
		defer unsub()

		p.Set(3, depth.StateKnown)

		if d, s := p.Depth(); d != 3 || s != depth.StateKnown {
			t.Fatalf("got (%d, %s)", d, s)
		}
		select {
		case v := <-ch:
			if v != 3 {
				t.Fatalf("got %d", v)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	})

	t.Run("set not known sends nothing", func(t *testing.T) {
		t.Parallel()

		p := mock.New()
		ch, unsub := p.Subscribe()
		defer unsub()

		p.Set(0, depth.StateNoHistory)

		select {
		case v := <-ch:
			t.Fatalf("unexpected %d", v)
		default:
		}
	})

	t.Run("options", func(t *testing.T) {
		t.Parallel()

		p := mock.New(mock.WithDepth(4), mock.WithState(depth.StateKnown))
		ch, unsub := p.Subscribe()
		defer unsub()

		select {
		case v := <-ch:
			if v != 4 {
				t.Fatalf("got %d", v)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	})
}

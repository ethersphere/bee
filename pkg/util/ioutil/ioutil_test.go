// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ioutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleReader(t *testing.T) {
	t.Parallel()

	t.Run("normal read", func(t *testing.T) {
		t.Parallel()

		data := "0123456789"
		timeout := 200 * time.Millisecond
		canceled := new(atomic.Bool)
		cancelFn := func(u uint64) { canceled.Store(true) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := IdleReader(ctx, strings.NewReader(data), timeout, cancelFn)

		buf, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		time.Sleep(2 * timeout)

		if canceled.Load() {
			t.Fatal("cancelFn called")
		}

		if want, have := data, string(buf); want != have {
			t.Fatalf("read data content mismatch: want: %s; have: %s", want, have)
		}

		_, err = io.ReadAll(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stuck read", func(t *testing.T) {
		t.Parallel()

		data := "0123456789"
		timeout := 200 * time.Millisecond
		canceled := new(atomic.Bool)
		cancelFn := func(u uint64) { canceled.Store(true) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := IdleReader(ctx, strings.NewReader(data), timeout, cancelFn)

		if canceled.Load() {
			t.Fatal("cancelFn called prematurely")
		}

		n, err := r.Read(make([]byte, 1))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want, have := 1, n; want != have {
			t.Fatalf("read data size mismatch: want: %d; have: %d", want, have)
		}

		time.Sleep(2 * timeout)

		if !canceled.Load() {
			t.Fatal("cancelFn not called")
		}
	})

	t.Run("EOF read", func(t *testing.T) {
		t.Parallel()

		timeout := 200 * time.Millisecond
		canceled := new(atomic.Bool)
		cancelFn := func(u uint64) { canceled.Store(true) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := IdleReader(ctx, new(bytes.Buffer), timeout, cancelFn)

		for i := 0; i < 10; i++ {
			n, err := r.Read(make([]byte, 1))
			if want, have := io.EOF, err; !errors.Is(have, want) {
				t.Fatalf("unexpected error: want: %v; have: %v", want, have)
			}

			if want, have := 0, n; want != have {
				t.Fatalf("read data size mismatch: want: %d; have: %d", want, have)
			}
		}

		time.Sleep(2 * timeout)

		if canceled.Load() {
			t.Fatal("cancelFn called")
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		t.Parallel()

		timeout := 200 * time.Millisecond
		canceled := new(atomic.Bool)
		cancelFn := func(u uint64) { canceled.Store(true) }
		ctx, cancel := context.WithCancel(context.Background())
		r := IdleReader(ctx, new(bytes.Buffer), timeout, cancelFn)

		if canceled.Load() {
			t.Fatal("cancelFn called prematurely")
		}

		cancel()

		n, err := r.Read(make([]byte, 1))
		if want, have := io.EOF, err; !errors.Is(have, want) {
			t.Fatalf("unexpected error: want: %v; have: %v", want, have)
		}

		if want, have := 0, n; want != have {
			t.Fatalf("read data size mismatch: want: %d; have: %d", want, have)
		}

		time.Sleep(2 * timeout)

		if canceled.Load() {
			t.Fatal("cancelFn called")
		}
	})
}

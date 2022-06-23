// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
)

func TestProgressReader(t *testing.T) {
	t.Run("normal read", func(t *testing.T) {
		read := uint64(0)
		data := "0123456789"
		done := 100 * time.Millisecond
		doFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := api.NewProgressReader(ctx, strings.NewReader(data), done, doFn)

		buf, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("doFn called prematurely: want: %d; have: %d", want, have)
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
		read := uint64(0)
		data := "0123456789"
		done := 100 * time.Millisecond
		doFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		api.NewProgressReader(ctx, strings.NewReader(data), done, doFn)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("doFn called prematurely: want: %d; have: %d", want, have)
		}

		time.Sleep(2 * done)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("doFn data length mismatch: want: %d; have: %d", want, have)
		}
	})
	t.Run("EOF read", func(t *testing.T) {
		read := uint64(0)
		done := 100 * time.Millisecond
		doFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := api.NewProgressReader(ctx, new(bytes.Buffer), done, doFn)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("doFn called prematurely: want: %d; have: %d", want, have)
		}

		n, err := r.Read(make([]byte, 1))
		if want, have := io.EOF, err; want != have {
			t.Fatalf("unexpected error: want: %v; have: %v", want, have)
		}

		if want, have := 0, n; want != have {
			t.Fatalf("read data size mismatch: want: %d; have: %d", want, have)
		}

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("doFn data length mismatch: want: %d; have: %d", want, have)
		}
	})
}

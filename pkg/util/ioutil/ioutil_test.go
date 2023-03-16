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

func TestTimeoutReader(t *testing.T) {
	t.Parallel()

	t.Run("normal read", func(t *testing.T) {
		t.Parallel()

		read := uint64(0)
		data := "0123456789"
		timeout := 100 * time.Millisecond
		cancelFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := TimeoutReader(ctx, strings.NewReader(data), timeout, cancelFn)

		buf, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("cancelFn called prematurely: want: %d; have: %d", want, have)
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

		read := uint64(0)
		data := "0123456789"
		timeout := 100 * time.Millisecond
		cancelFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		TimeoutReader(ctx, strings.NewReader(data), timeout, cancelFn)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("cancelFn called prematurely: want: %d; have: %d", want, have)
		}

		time.Sleep(2 * timeout)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("cancelFn data length mismatch: want: %d; have: %d", want, have)
		}
	})
	t.Run("EOF read", func(t *testing.T) {
		t.Parallel()

		read := uint64(0)
		timeout := 100 * time.Millisecond
		cancelFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := TimeoutReader(ctx, new(bytes.Buffer), timeout, cancelFn)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("cancelFn called prematurely: want: %d; have: %d", want, have)
		}

		n, err := r.Read(make([]byte, 1))
		if want, have := io.EOF, err; !errors.Is(have, want) {
			t.Fatalf("unexpected error: want: %v; have: %v", want, have)
		}

		if want, have := 0, n; want != have {
			t.Fatalf("read data size mismatch: want: %d; have: %d", want, have)
		}

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("cancelFn data length mismatch: want: %d; have: %d", want, have)
		}
	})

	t.Run("fail", func(t *testing.T) {
		timeout := 20 * time.Millisecond
		cancelFn := func(u uint64) {
			t.Fatalf("should not happen")
		}
		ctx := context.Background()
		data := "some data"

		r := TimeoutReader(ctx, strings.NewReader(data), timeout, cancelFn)

		buff := make([]byte, len([]byte(data)))
		r.Read(buff)

		if string(buff) != data {
			t.Fatalf("want: %v; have: %v", data, string(buff))
		}

		time.Sleep(2 * timeout)
	})
}

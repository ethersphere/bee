// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ioutil

import (
	"bytes"
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
		timeout := 10 * time.Millisecond
		callbackFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		r := TimeoutReader(strings.NewReader(data), timeout, callbackFn)

		buf, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("callbackFn called prematurely: want: %d; have: %d", want, have)
		}

		if want, have := data, string(buf); want != have {
			t.Fatalf("read data content mismatch: want: %s; have: %s", want, have)
		}
	})

	t.Run("stuck read", func(t *testing.T) {
		t.Parallel()

		read := uint64(0)
		data := "0123456789"
		timeout := 10 * time.Millisecond
		callbackFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		TimeoutReader(strings.NewReader(data), timeout, callbackFn)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("callbackFn called prematurely: want: %d; have: %d", want, have)
		}

		time.Sleep(2 * timeout)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("callbackFn data length mismatch: want: %d; have: %d", want, have)
		}
	})

	t.Run("EOF read", func(t *testing.T) {
		t.Parallel()

		read := uint64(0)
		timeout := 10 * time.Millisecond
		callbackFn := func(u uint64) { atomic.StoreUint64(&read, u) }
		r := TimeoutReader(new(bytes.Buffer), timeout, callbackFn)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("callbackFn called prematurely: want: %d; have: %d", want, have)
		}

		n, err := r.Read(make([]byte, 1))
		if want, have := io.EOF, err; !errors.Is(have, want) {
			t.Fatalf("unexpected error: want: %v; have: %v", want, have)
		}

		if want, have := 0, n; want != have {
			t.Fatalf("read data size mismatch: want: %d; have: %d", want, have)
		}

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("callbackFn data length mismatch: want: %d; have: %d", want, have)
		}
	})

	t.Run("timer stopped after read", func(t *testing.T) {
		t.Parallel()

		const data = "some data"
		const timeout = 10 * time.Millisecond

		callbackFn := func(u uint64) { t.Fatalf("should not happen") }
		r := TimeoutReader(strings.NewReader(data), timeout, callbackFn)

		buff, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err.Error())
		}

		if string(buff) != data {
			t.Fatalf("want: %v; have: %v", data, string(buff))
		}

		time.Sleep(2 * timeout)
	})
}

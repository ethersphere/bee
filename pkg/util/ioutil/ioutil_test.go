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

	const data = "some data"
	const timeout = 50 * time.Millisecond

	t.Run("timer stopped after read", func(t *testing.T) {
		t.Parallel()

		callbackFn := func(u uint64) { t.Fatalf("should not happen") }
		r := TimeoutReader(strings.NewReader(data), timeout, callbackFn)

		buff, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err.Error())
		}

		if want, have := data, string(buff); want != have {
			t.Fatalf("read data content mismatch: want: %s; have: %s", want, have)
		}

		time.Sleep(2 * timeout)
	})

	t.Run("read from drained reader", func(t *testing.T) {
		t.Parallel()

		callbackFn := func(u uint64) { t.Fatalf("should not happen") }
		r := TimeoutReader(strings.NewReader(data), timeout, callbackFn)

		buff, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err.Error())
		}

		time.Sleep(2 * timeout)

		if want, have := data, string(buff); want != have {
			t.Fatalf("read data content mismatch: want: %s; have: %s", want, have)
		}

		// Reading from reader again shouldn't read anything
		_, err = r.Read(make([]byte, len(data)))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("reader should be drained")
		}
	})

	t.Run("callback triggered after timeout", func(t *testing.T) {
		t.Parallel()

		read := uint64(0)
		callbackFn := func(u uint64) { atomic.StoreUint64(&read, 100) }
		TimeoutReader(strings.NewReader(data), timeout, callbackFn)

		if want, have := 0, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("callbackFn called prematurely: want: %d; have: %d", want, have)
		}

		time.Sleep(2 * timeout)

		if want, have := 100, int(atomic.LoadUint64(&read)); want != have {
			t.Fatalf("callbackFn data length mismatch: want: %d; have: %d", want, have)
		}
	})

	t.Run("EOF read", func(t *testing.T) {
		t.Parallel()

		read := uint64(0)
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
}

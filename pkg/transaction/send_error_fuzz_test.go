// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// FuzzTxMayBeInMempool checks classification of untrusted RPC error text.
func FuzzTxMayBeInMempool(f *testing.F) {
	for _, seed := range []string{
		"",
		"context deadline exceeded",
		"Context Deadline Exceeded",
		"context canceled",
		"connection reset by peer",
		"i/o timeout",
		"EOF",
		"unexpected EOF",
		"nonce too low",
		"execution reverted: AlreadyCommitted",
		"already known",
		"already imported",
		"connection refused",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, msg string) {
		got := transaction.TxMayBeInMempool(errors.New(msg))
		wrapped := &url.Error{Op: "Post", URL: "http://rpc", Err: errors.New(msg)}
		if transaction.TxMayBeInMempool(wrapped) != got {
			t.Fatalf("url.Error wrapper changed the result for %q", msg)
		}
		if !transaction.TxMayBeInMempool(fmt.Errorf("%s: %w", msg, context.DeadlineExceeded)) {
			t.Fatalf("wrapped deadline is not ambiguous for %q", msg)
		}
	})
}

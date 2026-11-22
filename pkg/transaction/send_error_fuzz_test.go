// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// FuzzTxMayBeInMempool checks classification of untrusted RPC error text.
// Only a lost response may leave the transaction in the mempool. A node answer,
// including "already known", must not.
func FuzzTxMayBeInMempool(f *testing.F) {
	f.Add("context deadline exceeded")
	f.Add("context canceled")
	f.Add("EOF")
	f.Add("unexpected EOF")
	f.Add("connection reset by peer")
	f.Add("i/o timeout")
	f.Add("nonce too low")
	f.Add("execution reverted: AlreadyCommitted")
	f.Add("already known")
	f.Add("already imported")
	f.Add("connection refused")
	f.Add("")

	maybe := []string{
		"contextdeadlineexceeded",
		"contextcanceled",
		"eof",
		"unexpectedeof",
		"connectionresetbypeer",
		"i/otimeout",
	}
	answered := []string{
		"noncetoolow",
		"executionreverted:alreadycommitted",
		"alreadyknown",
		"alreadyimported",
		"connectionrefused",
		"",
	}

	f.Fuzz(func(t *testing.T, msg string) {
		got := transaction.TxMayBeInMempool(errors.New(msg))
		normalized := strings.ToLower(strings.ReplaceAll(msg, " ", ""))
		for _, phrase := range maybe {
			if normalized == phrase && !got {
				t.Fatalf("%q may leave the transaction in the mempool", msg)
			}
		}
		for _, phrase := range answered {
			if normalized == phrase && got {
				t.Fatalf("%q is a node answer and must not be stored", msg)
			}
		}
	})
}

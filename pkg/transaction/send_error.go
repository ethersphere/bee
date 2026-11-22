// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"strings"
)

// txMayBeInMempool reports whether the transaction may be in the mempool.
// A nil error means the node accepted it. The transport errors below mean the
// RPC response was lost while the request was in flight.
// An EOF is a failed send: the transaction is not stored.
func txMayBeInMempool(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	msg := normalizeForMatch(err.Error())
	for _, needle := range mempoolMaybeNeedles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// mempoolMaybeNeedles are matched against a normalized error string.
// A hit means the RPC call ended without a node answer.
var mempoolMaybeNeedles = []string{
	"contextdeadlineexceeded",
	"contextcanceled",
	"connectionresetbypeer",
	"brokenpipe",
	"i/otimeout",
	"client.timeoutexceeded",
	"awaitingheaders",
	"useofclosednetworkconnection",
	"serverclosedidleconnection",
}

func normalizeForMatch(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
}

// isAlreadyKnown reports whether the node already has this exact transaction.
func isAlreadyKnown(err error) bool {
	if err == nil {
		return false
	}
	msg := normalizeForMatch(err.Error())
	return strings.Contains(msg, "alreadyknown") ||
		strings.Contains(msg, "alreadyimported") ||
		strings.Contains(msg, "knowntransaction")
}

func isReplacementUnderpriced(err error) bool {
	return err != nil && strings.Contains(normalizeForMatch(err.Error()), "replacementtransactionunderpriced")
}

func isNonceTooLow(err error) bool {
	if err == nil {
		return false
	}
	msg := normalizeForMatch(err.Error())
	return strings.Contains(msg, "noncetoolow") || strings.Contains(msg, "nonceistoolow")
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"fmt"
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

// rejectedBroadcast returns the error a caller should see when the node answered
// the broadcast. It returns nil when the transaction may be in the mempool.
func rejectedBroadcast(err error) error {
	switch {
	case err == nil:
		return nil
	case isAlreadyKnown(err):
		return ErrAlreadyImported
	case isReplacementUnderpriced(err):
		return fmt.Errorf("%w: %w", ErrReplacementUnderpriced, err)
	case isNonceTooLow(err):
		return fmt.Errorf("%w: %w", ErrNonceUsed, err)
	case !txMayBeInMempool(err):
		return err
	default:
		return nil
	}
}

// isAlreadyKnown reports whether the node already has this exact transaction.
func isAlreadyKnown(err error) bool {
	if err == nil {
		return false
	}
	msg := normalizeForMatch(err.Error())
	return strings.Contains(msg, "alreadyknown") ||
		strings.Contains(msg, "alreadyimported") ||
		(strings.Contains(msg, "knowntransaction") && !strings.Contains(msg, "unknowntransaction"))
}

func isReplacementUnderpriced(err error) bool {
	if err == nil {
		return false
	}
	msg := normalizeForMatch(err.Error())
	// Nethermind before 1.39 answers ReplacementNotAllowed instead of the geth phrase.
	// Its FeeTooLowToCompete is a full pool, not a same-nonce replacement.
	return strings.Contains(msg, "replacementtransactionunderpriced") ||
		strings.Contains(msg, "replacementnotallowed")
}

func isNonceTooLow(err error) bool {
	if err == nil {
		return false
	}
	// Nethermind reports OldNonce as "nonce too low", the same phrase as geth.
	msg := normalizeForMatch(err.Error())
	return strings.Contains(msg, "noncetoolow") || strings.Contains(msg, "nonceistoolow")
}

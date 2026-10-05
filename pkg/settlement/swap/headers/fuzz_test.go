// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	swap "github.com/ethersphere/bee/v2/pkg/settlement/swap/headers"
)

// FuzzParseSettlementResponseHeaders fuzzes the settlement response-header
// parser, which runs on the peer-supplied stream headers of a swap cheque
// exchange. The exchange-rate and deduction fields are decoded into big.Ints;
// the target asserts the parser never panics on arbitrary or missing fields.
func FuzzParseSettlementResponseHeaders(f *testing.F) {
	f.Add([]byte{0x01}, []byte{0x02})
	f.Add([]byte{}, []byte{})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff}, []byte(nil))

	f.Fuzz(func(t *testing.T, exchange, deduction []byte) {
		headers := p2p.Headers{
			"exchange":  exchange,
			"deduction": deduction,
		}
		ex, ded, err := swap.ParseSettlementResponseHeaders(headers)
		if err != nil {
			return
		}
		// on success both values must be non-negative big.Ints
		if ex.Sign() < 0 || ded.Sign() < 0 {
			t.Fatalf("negative header value: exchange=%s deduction=%s", ex, ded)
		}
	})
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"encoding/json"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
)

// FuzzSignedChequeUnmarshalJSON feeds arbitrary bytes to SignedCheque.UnmarshalJSON,
// which decodes cheques from peers and from the statestore. A cheque that
// decodes must have a non-negative payout and must survive a marshal and
// decode round trip unchanged.
func FuzzSignedChequeUnmarshalJSON(f *testing.F) {
	f.Add([]byte(goldenSignedCheque))
	f.Add([]byte(goldenCheque))
	f.Add([]byte(goldenNilSignature))
	f.Add([]byte(`{"CumulativePayout":0}`))
	f.Add([]byte(`{"CumulativePayout":115792089237316195423570985008687907853269984665640564039457584007913129639935}`))
	f.Add([]byte(`{"CumulativePayout":-1}`))
	f.Add([]byte(`{"CumulativePayout":null}`))
	f.Add([]byte(`{"CumulativePayout":"1"}`))
	f.Add([]byte(`{"Chequebook":"0x00","CumulativePayout":1}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		var cheque *chequebook.SignedCheque
		if err := json.Unmarshal(data, &cheque); err != nil || cheque == nil {
			return
		}

		if cheque.CumulativePayout == nil || cheque.CumulativePayout.Sign() < 0 {
			t.Fatalf("decoded cheque has invalid payout %v", cheque.CumulativePayout)
		}

		encoded, err := json.Marshal(cheque)
		if err != nil {
			t.Fatalf("marshal decoded cheque: %v", err)
		}

		var decoded chequebook.SignedCheque
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal re-encoded cheque %s: %v", encoded, err)
		}
		if !decoded.Equal(cheque) {
			t.Fatalf("round trip changed cheque: got %v, want %v", &decoded, cheque)
		}
	})
}

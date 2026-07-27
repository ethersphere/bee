// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/pss"
)

// FuzzParseRecipient fuzzes pss.ParseRecipient, which decodes an
// attacker-supplied recipient public-key hex string (from the /pss send API and
// PSS addressing) via hex.DecodeString + btcec.ParsePubKey. This is the same
// parser reached through the API mapStructure hook, but there mapStructure's
// top-level recover() would mask any panic; here it is exercised unmasked so a
// real panic in the decode path would surface. The target asserts it never
// panics and that a nil error implies a non-nil public key.
func FuzzParseRecipient(f *testing.F) {
	f.Add("03c9d2b1e2a1b0f4c8d6e5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3")
	f.Add("")
	f.Add("zz")
	f.Add("04")

	f.Fuzz(func(t *testing.T, recipient string) {
		pk, err := pss.ParseRecipient(recipient)
		if err != nil {
			return
		}
		if pk == nil {
			t.Fatal("ParseRecipient returned nil error but nil public key")
		}
	})
}

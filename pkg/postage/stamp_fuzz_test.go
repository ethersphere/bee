// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/postage"
)

// FuzzStampUnmarshalBinary fuzzes the postage-stamp wire decoder, which runs on
// the peer-supplied Stamp field of every pushsync and pullsync delivery. It
// asserts the decoder only accepts inputs of the exact stamp size, that a
// successful decode round-trips byte-for-byte, and that hashing an accepted
// stamp never fails or panics (Stamp.Hash is called on this decoded value while
// matching deliveries against wanted chunks).
func FuzzStampUnmarshalBinary(f *testing.F) {
	f.Add(make([]byte, postage.StampSize)) // exact size
	f.Add([]byte{})                        // empty
	f.Add(make([]byte, postage.StampSize-1))
	f.Add(make([]byte, postage.StampSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		s := new(postage.Stamp)
		if err := s.UnmarshalBinary(data); err != nil {
			return
		}

		if len(data) != postage.StampSize {
			t.Fatalf("accepted stamp of length %d, want exactly %d", len(data), postage.StampSize)
		}

		out, err := s.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal after successful unmarshal: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatal("marshal/unmarshal is not a byte-for-byte round-trip")
		}

		if _, err := s.Hash(); err != nil {
			t.Fatalf("hash of an accepted stamp: %v", err)
		}
	})
}

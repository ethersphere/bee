// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzCountersUnmarshalJSON exercises the persisted peer-metrics decoder
// Counters.UnmarshalJSON, which json.Unmarshal's a leveldb-stored blob into an
// unexported persistentCounters and copies the four persisted fields into the
// Counters struct. It transitively exercises swarm.Address.UnmarshalJSON.
func FuzzCountersUnmarshalJSON(f *testing.F) {
	// Valid seed built the production way: construct a real Counters, marshal it.
	seed := &Counters{
		peerAddress:       swarm.NewAddress([]byte{0x01, 0x02, 0x03, 0x04}),
		lastSeenTimestamp: 1234567890,
		connTotalDuration: 42 * time.Second,
		IsBootnode:        true,
	}
	if b, err := seed.MarshalJSON(); err != nil {
		f.Fatal(err)
	} else {
		f.Add(b)
	}

	// Benign edge buffers.
	f.Add([]byte(nil))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var cs Counters
		// Primary invariant: never panic on arbitrary bytes.
		err := cs.UnmarshalJSON(data)
		if err != nil {
			return
		}

		// Round-trip only on success: MarshalJSON->UnmarshalJSON must be stable
		// across the four persisted fields.
		b, err := cs.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON after successful UnmarshalJSON failed: %v", err)
		}

		var cs2 Counters
		if err := cs2.UnmarshalJSON(b); err != nil {
			t.Fatalf("re-UnmarshalJSON of marshaled bytes failed: %v", err)
		}

		if !cs.peerAddress.Equal(cs2.peerAddress) {
			t.Fatalf("peerAddress mismatch: %v != %v", cs.peerAddress, cs2.peerAddress)
		}
		if cs.lastSeenTimestamp != cs2.lastSeenTimestamp {
			t.Fatalf("lastSeenTimestamp mismatch: %d != %d", cs.lastSeenTimestamp, cs2.lastSeenTimestamp)
		}
		if cs.connTotalDuration != cs2.connTotalDuration {
			t.Fatalf("connTotalDuration mismatch: %d != %d", cs.connTotalDuration, cs2.connTotalDuration)
		}
		if cs.IsBootnode != cs2.IsBootnode {
			t.Fatalf("IsBootnode mismatch: %v != %v", cs.IsBootnode, cs2.IsBootnode)
		}
	})
}

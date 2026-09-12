// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
)

func hiveFrame(tb testing.TB, msg protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzPeersRead fuzzes the full hive gossip ingestion decode chain: the
// length-delimited protobuf reader that parses a Peers message, followed by the
// per-entry underlay and signed-address decoders invoked by checkAndAddPeers.
// Every byte is peer-supplied, so the target asserts the whole chain never
// panics on malformed or truncated input.
func FuzzPeersRead(f *testing.F) {
	const networkID = uint64(1)

	f.Add(hiveFrame(f, &pb.Peers{Peers: []*pb.BzzAddress{{
		Underlay:          make([]byte, 8),
		Overlay:           make([]byte, 32),
		Signature:         make([]byte, 65),
		Nonce:             make([]byte, bzz.NonceLength),
		Timestamp:         1,
		ChequebookAddress: make([]byte, 20),
	}}}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var peers pb.Peers
		if err := r.ReadMsg(&peers); err != nil {
			return
		}

		for _, p := range peers.Peers {
			if p == nil {
				continue
			}
			// exactly the decoders checkAndAddPeers runs on each entry
			_, _ = bzz.DeserializeUnderlays(p.Underlay)
			_, _ = bzz.ParseAddress(p.Underlay, p.Overlay, p.Signature, p.Nonce, p.Timestamp, networkID, p.ChequebookAddress)
		}
	})
}

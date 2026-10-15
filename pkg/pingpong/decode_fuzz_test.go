// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pingpong_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pingpong/pb"
)

// pingFrame marshals a Ping into the length-delimited protobuf wire form the
// pingpong handler reads at pingpong.go:114.
func pingFrame(f *testing.F, m *pb.Ping) []byte {
	f.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(m); err != nil {
		f.Fatal(err)
	}
	return buf.Bytes()
}

// pongFrame marshals a Pong into the length-delimited protobuf wire form the
// pingpong client reads at pingpong.go:92.
func pongFrame(f *testing.F, m *pb.Pong) []byte {
	f.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(m); err != nil {
		f.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzPingDecode fuzzes the peer-controlled wire-decode of pb.Ping — exactly the
// length-delimited protobuf read the pingpong handler performs on bytes a remote
// peer writes into the stream (pingpong.go:114). It asserts the decoder never
// panics and, on a successful decode, that re-marshaling and decoding again is
// stable (round-trip preserves the Greeting).
func FuzzPingDecode(f *testing.F) {
	f.Add(pingFrame(f, &pb.Ping{Greeting: "hey"}))
	f.Add(pingFrame(f, &pb.Ping{}))
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var m pb.Ping
		if err := r.ReadMsg(&m); err != nil {
			return
		}

		var buf bytes.Buffer
		if err := protobuf.NewWriter(&buf).WriteMsg(&m); err != nil {
			t.Fatalf("re-marshal decoded Ping: %v", err)
		}
		var got pb.Ping
		if err := protobuf.NewReader(bytes.NewReader(buf.Bytes())).ReadMsg(&got); err != nil {
			t.Fatalf("re-decode marshaled Ping: %v", err)
		}
		if got.Greeting != m.Greeting {
			t.Fatalf("round-trip greeting mismatch: got %q want %q", got.Greeting, m.Greeting)
		}
	})
}

// FuzzPongDecode fuzzes the peer-controlled wire-decode of pb.Pong — the read a
// pinging node performs on the reply bytes a malicious responding peer sends
// back (pingpong.go:92). It asserts the decoder never panics and, on success,
// that the decoded Response round-trips.
func FuzzPongDecode(f *testing.F) {
	f.Add(pongFrame(f, &pb.Pong{Response: "{hey}"}))
	f.Add(pongFrame(f, &pb.Pong{}))
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var m pb.Pong
		if err := r.ReadMsg(&m); err != nil {
			return
		}

		var buf bytes.Buffer
		if err := protobuf.NewWriter(&buf).WriteMsg(&m); err != nil {
			t.Fatalf("re-marshal decoded Pong: %v", err)
		}
		var got pb.Pong
		if err := protobuf.NewReader(bytes.NewReader(buf.Bytes())).ReadMsg(&got); err != nil {
			t.Fatalf("re-decode marshaled Pong: %v", err)
		}
		if got.Response != m.Response {
			t.Fatalf("round-trip response mismatch: got %q want %q", got.Response, m.Response)
		}
	})
}

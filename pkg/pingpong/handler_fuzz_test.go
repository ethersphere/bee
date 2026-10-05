// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pingpong_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/pingpong"
	"github.com/ethersphere/bee/v2/pkg/pingpong/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var fuzzHandlerAddr = swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")

// streamtestPongCap is the highest number of handler-written Pongs the
// streamtest recorder can hold before it wedges. record.Write appends the bytes
// and then sends on a size-16 signal channel *while holding the record lock*
// (pkg/p2p/streamtest/streamtest.go); when that channel is full the write blocks
// with the lock still held, and any reader deadlocks on bytesSize(), which needs
// the same lock. The pingpong handler emits one Pong per decoded Ping, so an
// input decoding to more than 16 frames would drive the handler past this cap
// and deadlock the *test harness* — not the node. Real libp2p streams provide
// flow control, so this bound is a streamtest limitation, not a Bee property.
const streamtestPongCap = 16

// countPingFrames reports how many length-delimited pb.Ping frames the handler
// will successfully decode from data before it hits an error or EOF, using the
// same protobuf.Reader the handler uses. It equals the number of Pong replies
// the handler will emit; the real decode still happens inside the handler under
// test — this is only used to keep the drive within the recorder's capacity.
func countPingFrames(data []byte) int {
	r := protobuf.NewReader(bytes.NewReader(data))
	n := 0
	for {
		var p pb.Ping
		if err := r.ReadMsg(&p); err != nil {
			return n
		}
		n++
	}
}

// FuzzHandler drives the real pingpong Service.handler (pingpong.go:104)
// end-to-end over the streamtest recorder, feeding it arbitrary peer bytes. It
// exercises the full server read path: length-delimited protobuf framing, decode
// into pb.Ping, and the Pong write-back ("{" + Greeting + "}"). This is the true
// trust boundary — a remote peer opening the pingpong stream and writing hostile
// bytes.
//
// Inputs decoding to more than streamtestPongCap frames are skipped: they would
// deadlock the streamtest recorder (see streamtestPongCap), not the node, so
// driving them would only test the harness. Within that bound the handler writes
// all its Pongs into the recorder's buffer without ever blocking, so after
// closing the write side (which delivers EOF and lets the handler loop exit) the
// full Pong output is available from the record.
//
// The handler must not panic. Every Pong it emits must be brace-wrapped, which
// always holds because the handler builds it as "{" + ping.Greeting + "}".
func FuzzHandler(f *testing.F) {
	f.Add(pingFrame(f, &pb.Ping{Greeting: "hey"}))
	f.Add(pingFrame(f, &pb.Ping{}))
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		if countPingFrames(data) > streamtestPongCap {
			// Skip inputs that would overrun the streamtest recorder's bounded
			// buffer and deadlock the harness (not the node). The per-frame
			// decode is identical regardless of frame count, so coverage of the
			// real trust boundary is unaffected.
			return
		}

		server := pingpong.New(nil, log.Noop, nil)

		recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

		stream, err := recorder.NewStream(t.Context(), fuzzHandlerAddr, nil, "pingpong", "1.0.0", "pingpong")
		if err != nil {
			t.Fatalf("new stream: %v", err)
		}

		if _, err := stream.Write(data); err != nil {
			_ = stream.Close()
			return
		}
		// Closing the write side hands the handler an EOF so its read loop exits;
		// without this the handler blocks forever waiting for the next frame.
		_ = stream.Close()

		// Records blocks until the handler goroutine has returned, so the Out()
		// buffer below is complete and the handler is guaranteed to have run.
		records, err := recorder.Records(fuzzHandlerAddr, "pingpong", "1.0.0", "pingpong")
		if err != nil {
			t.Fatalf("records: %v", err)
		}

		for _, rec := range records {
			msgs, err := protobuf.ReadMessages(
				bytes.NewReader(rec.Out()),
				func() protobuf.Message { return new(pb.Pong) },
			)
			if err != nil {
				// The handler only ever writes whole Pong frames, so a decode
				// error here cannot come from valid handler output.
				t.Fatalf("read pong messages: %v", err)
			}
			for _, m := range msgs {
				resp := m.(*pb.Pong).Response
				// The handler wraps the greeting in braces, so every Pong it
				// returns is brace-delimited regardless of the (possibly empty)
				// greeting.
				if !strings.HasPrefix(resp, "{") || !strings.HasSuffix(resp, "}") {
					t.Fatalf("pong response %q is not brace-wrapped", resp)
				}
			}
		}
	})
}

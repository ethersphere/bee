// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestGreet(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	server := bps.New(nil, logger)

	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	client := bps.New(recorder, logger)

	addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	greeting := "world"

	response, err := client.Greet(context.Background(), addr, greeting)
	if err != nil {
		t.Fatal(err)
	}

	if want := "hello, world"; response != want {
		t.Fatalf("got response %q, want %q", response, want)
	}

	records, err := recorder.Records(addr, "bps", "1.0.0", "bps")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Hello) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if l := len(messages); l != 1 {
		t.Fatalf("got %v messages, want %v", l, 1)
	}
	if got := messages[0].(*pb.Hello).Greeting; got != greeting {
		t.Fatalf("got greeting %q, want %q", got, greeting)
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.Welcome) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if l := len(messages); l != 1 {
		t.Fatalf("got %v messages, want %v", l, 1)
	}
	if got, want := messages[0].(*pb.Welcome).Response, "hello, world"; got != want {
		t.Fatalf("got response %q, want %q", got, want)
	}

	if err := record.Err(); err != nil {
		t.Fatal(err)
	}
}

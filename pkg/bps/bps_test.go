// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	server := bps.New(nil, logger)

	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	client := bps.New(recorder, logger)

	addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	topic := []byte{0, 1, 2, 3, 4}
	// greeting := "world"

	challenge, _, err := client.Join(context.Background(), addr, topic)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(challenge)
	records, err := recorder.Records(addr, "bps", "1.0.0", "bps")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 2)
	}
	record := records[0]

	// client -> server: SystemMessage{Join}
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.SystemMessage) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if l := len(messages); l != 1 {
		t.Fatalf("got %v messages, want %v", l, 1)
	}
	join := messages[0].(*pb.SystemMessage).GetJoin()
	if join == nil {
		t.Fatal("expected join message")
	}
	if !bytes.Equal(join.Topic, topic) {
		t.Fatalf("got topic %x, want %x", join.Topic, topic)
	}

	// server -> client returns challenge
	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.JoinAck) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if l := len(messages); l != 1 {
		t.Fatalf("got %v messages, want %v", l, 1)
	}
	cl := messages[0].(*pb.JoinAck).Challenge
	if cl == nil {
		t.Fatal("expected joinack message")
	}
	if !bytes.Equal(challenge, cl) {
		t.Fatalf("got challenge %x, want %x", cl, challenge)
	}
}

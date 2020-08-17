// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pingpong_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"runtime"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/pingpong/pb"
)

func TestPing(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	// create a pingpong server that handles the incoming stream
	server := pingpong.New(nil, logger, nil)

	// setup the stream recorder to record stream data
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			if runtime.GOOS == "windows" {
				// windows has a bit lower time resolution
				// so, slow down the handler with a middleware
				// not to get 0s for rtt value
				time.Sleep(100 * time.Millisecond)
			}
			return f
		}),
	)

	// create a pingpong client that will do pinging
	client := pingpong.New(recorder, logger, nil)

	// ping
	addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	greetings := []string{"hey", "there", "fella"}
	rtt, err := client.Ping(context.Background(), addr, greetings...)
	if err != nil {
		t.Fatal(err)
	}

	// check that RTT is a sane value
	if rtt <= 0 {
		t.Errorf("invalid RTT value %v", rtt)
	}

	// get a record for this stream
	records, err := recorder.Records(addr, "pingpong", "1.0.0", "pingpong")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]

	// validate received ping greetings from the client
	wantGreetings := greetings
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Ping) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotGreetings []string
	for _, m := range messages {
		gotGreetings = append(gotGreetings, m.(*pb.Ping).Greeting)
	}
	if fmt.Sprint(gotGreetings) != fmt.Sprint(wantGreetings) {
		t.Errorf("got greetings %v, want %v", gotGreetings, wantGreetings)
	}

	// validate sent pong responses by handler
	var wantResponses []string
	for _, g := range greetings {
		wantResponses = append(wantResponses, "{"+g+"}")
	}
	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.Pong) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotResponses []string
	for _, m := range messages {
		gotResponses = append(gotResponses, m.(*pb.Pong).Response)
	}
	if fmt.Sprint(gotResponses) != fmt.Sprint(wantResponses) {
		t.Errorf("got responses %v, want %v", gotResponses, wantResponses)
	}

	if err := record.Err(); err != nil {
		t.Fatal(err)
	}
}

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

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/pingpong/pb"
)

func TestPing(t *testing.T) {
	logger := logging.New(ioutil.Discard)

	// create a pingpong server that handles the incoming stream
	server := pingpong.New(pingpong.Options{
		Logger: logger,
	})

	// setup the stream recorder to record stream data
	recorder := mock.NewRecorder(
		mock.WithProtocols(server.Protocol()),
		mock.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
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
	client := pingpong.New(pingpong.Options{
		Streamer: recorder,
		Logger:   logger,
	})

	// ping
	peerID := "124"
	greetings := []string{"hey", "there", "fella"}
	rtt, err := client.Ping(context.Background(), peerID, greetings...)
	if err != nil {
		t.Fatal(err)
	}

	// check that RTT is a sane value
	if rtt <= 0 {
		t.Errorf("invalid RTT value %v", rtt)
	}

	// get a record for this stream
	records, err := recorder.Records(peerID, "pingpong", "pingpong", "1.0.0")
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
}

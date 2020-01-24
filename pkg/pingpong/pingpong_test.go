// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pingpong_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pingpong"
)

func TestPing(t *testing.T) {
	logger := logging.New(ioutil.Discard)

	// create a pingpong server that handles the incoming stream
	server := pingpong.New(pingpong.Options{
		Logger: logger,
	})

	// setup the stream recorder to record stream data
	recorder := mock.NewRecorder(server.Protocol())

	// create a pingpong client that will do pinging
	client := pingpong.New(pingpong.Options{
		Streamer: recorder,
		Logger:   logger,
	})

	// ping
	peerID := "/p2p/QmZt98UimwpW9ptJumKTq7B7t3FzNfyoWVNGcd8PFCd7XS"
	greetings := []string{"hey", "there", "fella"}
	_, err := client.Ping(context.Background(), peerID, greetings...)
	if err != nil {
		t.Fatal(err)
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
		func() protobuf.Message { return new(pingpong.Ping) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotGreetings []string
	for _, m := range messages {
		gotGreetings = append(gotGreetings, m.(*pingpong.Ping).Greeting)
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
		func() protobuf.Message { return new(pingpong.Pong) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotResponses []string
	for _, m := range messages {
		gotResponses = append(gotResponses, m.(*pingpong.Pong).Response)
	}
	if fmt.Sprint(gotResponses) != fmt.Sprint(wantResponses) {
		t.Errorf("got responses %v, want %v", gotResponses, wantResponses)
	}
}

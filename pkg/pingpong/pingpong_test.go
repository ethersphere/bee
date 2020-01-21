package pingpong_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/janos/bee/pkg/p2p/mock"
	"github.com/janos/bee/pkg/p2p/protobuf"
	"github.com/janos/bee/pkg/pingpong"
)

func TestPing(t *testing.T) {
	// create a pingpong server that handles the incoming stream
	server := pingpong.New(nil)

	// setup the stream recorder to record stream data
	recorder := mock.NewRecorder(server.Protocol())

	// create a pingpong client that will do pinging
	client := pingpong.New(recorder)

	// ping
	peerID := "/p2p/QmZt98UimwpW9ptJumKTq7B7t3FzNfyoWVNGcd8PFCd7XS"
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

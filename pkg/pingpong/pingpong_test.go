package pingpong_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/janos/bee/pkg/p2p/mock"
	"github.com/janos/bee/pkg/p2p/protobuf"
	"github.com/janos/bee/pkg/pingpong"
)

func TestPing(t *testing.T) {
	// create a pingpong server that handles the incoming stream
	server := pingpong.New(nil)

	// setup the mock streamer to record stream data
	streamer := mock.NewStreamer(server.Handler)

	// create a pingpong client that will do pinging
	client := pingpong.New(streamer)

	// ping
	greetings := []string{"hey", "there", "fella"}
	rtt, err := client.Ping(context.Background(), "/p2p/QmZt98UimwpW9ptJumKTq7B7t3FzNfyoWVNGcd8PFCd7XS", greetings...)
	if err != nil {
		t.Fatal(err)
	}

	// check that RTT is a sane value
	if rtt <= 0 {
		t.Errorf("invalid RTT value %v", rtt)
	}

	// validate received ping greetings
	r := protobuf.NewReader(bytes.NewReader(streamer.In.Bytes()))
	var gotGreetings []string
	for {
		var ping pingpong.Ping
		if err := r.ReadMsg(&ping); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		gotGreetings = append(gotGreetings, ping.Greeting)
	}
	if fmt.Sprint(gotGreetings) != fmt.Sprint(greetings) {
		t.Errorf("got greetings %v, want %v", gotGreetings, greetings)
	}

	// validate send pong responses by handler
	r = protobuf.NewReader(bytes.NewReader(streamer.Out.Bytes()))
	var wantResponses []string
	for _, g := range greetings {
		wantResponses = append(wantResponses, "{"+g+"}")
	}
	var gotResponses []string
	for {
		var pong pingpong.Pong
		if err := r.ReadMsg(&pong); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		gotResponses = append(gotResponses, pong.Response)
	}
	if fmt.Sprint(gotResponses) != fmt.Sprint(wantResponses) {
		t.Errorf("got responses %v, want %v", gotResponses, wantResponses)
	}
}

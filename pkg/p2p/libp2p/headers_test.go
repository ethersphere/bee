// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestHeaders(t *testing.T) {
	headers := p2p.Headers{
		"test-header-key": []byte("header-value"),
		"other-key":       []byte("other-value"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	var gotHeaders p2p.Headers
	handled := make(chan struct{})
	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		if ctx == nil {
			t.Fatal("missing context")
		}
		if !p.Address.Equal(overlay2) {
			t.Fatalf("got peer %v, want %v", p.Address, overlay2)
		}
		gotHeaders = stream.Headers()
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	stream, err := s2.NewStream(ctx, overlay1, headers, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	if fmt.Sprint(gotHeaders) != fmt.Sprint(headers) {
		t.Errorf("got headers %+v, want %+v", gotHeaders, headers)
	}
}

func TestHeaders_empty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{})

	var gotHeaders p2p.Headers
	handled := make(chan struct{})
	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		if ctx == nil {
			t.Fatal("missing context")
		}
		if !p.Address.Equal(overlay2) {
			t.Fatalf("got peer %v, want %v", p.Address, overlay2)
		}
		gotHeaders = stream.Headers()
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	stream, err := s2.NewStream(ctx, overlay1, nil, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	if len(gotHeaders) != 0 {
		t.Errorf("got headers %+v, want none", gotHeaders)
	}
}

func TestHeadler(t *testing.T) {
	receivedHeaders := p2p.Headers{
		"test-header-key": []byte("header-value"),
		"other-key":       []byte("other-value"),
	}
	sentHeaders := p2p.Headers{
		"sent-header-key": []byte("sent-value"),
		"other-sent-key":  []byte("other-sent-value"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{})

	s2, _ := newService(t, 1, libp2pServiceOpts{})

	var gotReceivedHeaders p2p.Headers
	handled := make(chan struct{})
	if err := s1.AddProtocol(p2p.ProtocolSpec{
		Name:    testProtocolName,
		Version: testProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name: testStreamName,
				Handler: func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
					return nil
				},
				Headler: func(headers p2p.Headers, address swarm.Address) p2p.Headers {
					defer close(handled)
					gotReceivedHeaders = headers
					return sentHeaders
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	stream, err := s2.NewStream(ctx, overlay1, receivedHeaders, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	if fmt.Sprint(gotReceivedHeaders) != fmt.Sprint(receivedHeaders) {
		t.Errorf("got received headers %+v, want %+v", gotReceivedHeaders, receivedHeaders)
	}

	gotSentHeaders := stream.Headers()
	if fmt.Sprint(gotSentHeaders) != fmt.Sprint(sentHeaders) {
		t.Errorf("got sent headers %+v, want %+v", gotSentHeaders, sentHeaders)
	}
}

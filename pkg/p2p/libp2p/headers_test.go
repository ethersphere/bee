// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestHeaders_BothSupportTrace tests header exchange when both peers support trace headers
func TestHeaders_BothSupportTrace(t *testing.T) {
	t.Parallel()

	headers := p2p.Headers{
		"test-header-key": []byte("header-value"),
		"other-key":       []byte("other-value"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode:           true,
		EnableTraceHeaders: true,
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		EnableTraceHeaders: true,
	}})

	var gotHeaders p2p.Headers
	handled := make(chan struct{})
	testMessage := []byte("test-message")
	receivedMessage := make(chan []byte, 1)

	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		if ctx == nil {
			t.Fatal("missing context")
		}
		if !p.Address.Equal(overlay2) {
			t.Fatalf("got peer %v, want %v", p.Address, overlay2)
		}
		gotHeaders = stream.Headers()

		// Read test message from stream
		buf := make([]byte, len(testMessage))
		_, err := stream.Read(buf)
		if err != nil {
			t.Errorf("failed to read from stream: %v", err)
		}
		receivedMessage <- buf
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	// Wait for handshake to complete and capabilities to be registered
	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	stream, err := s2.NewStream(ctx, overlay1, headers, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Send test message to verify stream is working
	_, err = stream.Write(testMessage)
	if err != nil {
		t.Errorf("failed to write to stream: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Verify message was received
	select {
	case msg := <-receivedMessage:
		if string(msg) != string(testMessage) {
			t.Errorf("got message %s, want %s", string(msg), string(testMessage))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	if fmt.Sprint(gotHeaders) != fmt.Sprint(headers) {
		t.Errorf("got headers %+v, want %+v", gotHeaders, headers)
	}
}

// TestHeaders_BothSupportTrace_EmptyHeaders tests empty header exchange when both peers support trace headers
func TestHeaders_BothSupportTrace_EmptyHeaders(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode:           true,
		EnableTraceHeaders: true,
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		EnableTraceHeaders: true,
	}})

	var gotHeaders p2p.Headers
	handled := make(chan struct{})
	testMessage := []byte("test-message-empty-headers")
	receivedMessage := make(chan []byte, 1)

	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		if ctx == nil {
			t.Fatal("missing context")
		}
		if !p.Address.Equal(overlay2) {
			t.Fatalf("got peer %v, want %v", p.Address, overlay2)
		}
		gotHeaders = stream.Headers()

		// Read test message from stream
		buf := make([]byte, len(testMessage))
		_, err := stream.Read(buf)
		if err != nil {
			t.Errorf("failed to read from stream: %v", err)
		}
		receivedMessage <- buf
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	// Wait for handshake to complete and capabilities to be registered
	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	stream, err := s2.NewStream(ctx, overlay1, nil, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Send test message to verify stream is working
	_, err = stream.Write(testMessage)
	if err != nil {
		t.Errorf("failed to write to stream: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Verify message was received
	select {
	case msg := <-receivedMessage:
		if string(msg) != string(testMessage) {
			t.Errorf("got message %s, want %s", string(msg), string(testMessage))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	if len(gotHeaders) != 0 {
		t.Errorf("got headers %+v, want none", gotHeaders)
	}
}

// TestHeaders_LocalSupportRemoteNoSupport tests no header exchange when local supports but remote doesn't
func TestHeaders_LocalSupportRemoteNoSupport(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode:           true,
		EnableTraceHeaders: true, // Local supports
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		EnableTraceHeaders: false, // Remote doesn't support
	}})

	var gotHeaders p2p.Headers
	handled := make(chan struct{})
	testMessage := []byte("test-message-mixed-caps-1")
	receivedMessage := make(chan []byte, 1)

	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		if ctx == nil {
			t.Fatal("missing context")
		}
		if !p.Address.Equal(overlay2) {
			t.Fatalf("got peer %v, want %v", p.Address, overlay2)
		}
		gotHeaders = stream.Headers()

		// Read test message from stream
		buf := make([]byte, len(testMessage))
		_, err := stream.Read(buf)
		if err != nil {
			t.Errorf("failed to read from stream: %v", err)
		}
		receivedMessage <- buf
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	// Wait for handshake to complete and capabilities to be registered
	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	// Try to send headers but they should be ignored due to capability mismatch
	providedHeaders := p2p.Headers{
		"ignored-header": []byte("ignored-value"),
	}

	stream, err := s2.NewStream(ctx, overlay1, providedHeaders, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Send test message to verify stream is working
	_, err = stream.Write(testMessage)
	if err != nil {
		t.Errorf("failed to write to stream: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Verify message was received
	select {
	case msg := <-receivedMessage:
		if string(msg) != string(testMessage) {
			t.Errorf("got message %s, want %s", string(msg), string(testMessage))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	// No headers should be exchanged due to capability mismatch
	if len(gotHeaders) != 0 {
		t.Errorf("expected no headers due to capability mismatch, got %+v", gotHeaders)
	}

	// Verify that response headers are also empty
	responseHeaders := stream.ResponseHeaders()
	if len(responseHeaders) != 0 {
		t.Errorf("expected no response headers when capabilities don't match, got %+v", responseHeaders)
	}
}

// TestHeaders_LocalNoSupportRemoteSupport tests no header exchange when local doesn't support but remote does
func TestHeaders_LocalNoSupportRemoteSupport(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode:           true,
		EnableTraceHeaders: false, // Local doesn't support
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		EnableTraceHeaders: true, // Remote supports
	}})

	var gotHeaders p2p.Headers
	handled := make(chan struct{})
	testMessage := []byte("test-message-mixed-caps-2")
	receivedMessage := make(chan []byte, 1)

	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		if ctx == nil {
			t.Fatal("missing context")
		}
		if !p.Address.Equal(overlay2) {
			t.Fatalf("got peer %v, want %v", p.Address, overlay2)
		}
		gotHeaders = stream.Headers()

		// Read test message from stream
		buf := make([]byte, len(testMessage))
		_, err := stream.Read(buf)
		if err != nil {
			t.Errorf("failed to read from stream: %v", err)
		}
		receivedMessage <- buf
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	// Wait for handshake to complete and capabilities to be registered
	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	// Try to send headers but they should be ignored due to capability mismatch
	providedHeaders := p2p.Headers{
		"another-ignored-header": []byte("another-ignored-value"),
	}

	stream, err := s2.NewStream(ctx, overlay1, providedHeaders, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Send test message to verify stream is working
	_, err = stream.Write(testMessage)
	if err != nil {
		t.Errorf("failed to write to stream: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Verify message was received
	select {
	case msg := <-receivedMessage:
		if string(msg) != string(testMessage) {
			t.Errorf("got message %s, want %s", string(msg), string(testMessage))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	// No headers should be exchanged due to capability mismatch
	if len(gotHeaders) != 0 {
		t.Errorf("expected no headers due to capability mismatch, got %+v", gotHeaders)
	}
}

// TestHeaders_BothNoSupport tests no header exchange when both peers don't support trace headers
func TestHeaders_BothNoSupport(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode:           true,
		EnableTraceHeaders: false, // Local doesn't support
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		EnableTraceHeaders: false, // Remote doesn't support
	}})

	var gotHeaders p2p.Headers
	handled := make(chan struct{})
	testMessage := []byte("test-message-both-no-support")
	receivedMessage := make(chan []byte, 1)

	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
		if ctx == nil {
			t.Fatal("missing context")
		}
		if !p.Address.Equal(overlay2) {
			t.Fatalf("got peer %v, want %v", p.Address, overlay2)
		}
		gotHeaders = stream.Headers()

		// Read test message from stream
		buf := make([]byte, len(testMessage))
		_, err := stream.Read(buf)
		if err != nil {
			t.Errorf("failed to read from stream: %v", err)
		}
		receivedMessage <- buf
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	if _, err := s2.Connect(ctx, addr); err != nil {
		t.Fatal(err)
	}

	// Wait for handshake to complete and capabilities to be registered
	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	// Headers provided but should be completely ignored
	providedHeaders := p2p.Headers{
		"completely-ignored": []byte("completely-ignored-value"),
	}

	stream, err := s2.NewStream(ctx, overlay1, providedHeaders, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Send test message to verify stream is working
	_, err = stream.Write(testMessage)
	if err != nil {
		t.Errorf("failed to write to stream: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Verify message was received
	select {
	case msg := <-receivedMessage:
		if string(msg) != string(testMessage) {
			t.Errorf("got message %s, want %s", string(msg), string(testMessage))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	// No headers should be exchanged since both don't support trace headers
	if len(gotHeaders) != 0 {
		t.Errorf("expected no headers when both peers don't support trace headers, got %+v", gotHeaders)
	}
}

// TestHeadler tests header exchange with Headler function when both peers support trace headers
func TestHeadler(t *testing.T) {
	t.Parallel()

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

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode:           true,
		EnableTraceHeaders: true,
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		EnableTraceHeaders: true,
	}})

	var gotReceivedHeaders p2p.Headers
	handled := make(chan struct{})
	testMessage := []byte("test-message-headler")
	receivedMessage := make(chan []byte, 1)

	if err := s1.AddProtocol(p2p.ProtocolSpec{
		Name:    testProtocolName,
		Version: testProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name: testStreamName,
				Handler: func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
					// Read test message from stream
					buf := make([]byte, len(testMessage))
					_, err := stream.Read(buf)
					if err != nil {
						t.Errorf("failed to read from stream: %v", err)
					}
					receivedMessage <- buf
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

	// Wait for handshake to complete and capabilities to be registered
	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	stream, err := s2.NewStream(ctx, overlay1, receivedHeaders, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Send test message to verify stream is working
	_, err = stream.Write(testMessage)
	if err != nil {
		t.Errorf("failed to write to stream: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Verify message was received
	select {
	case msg := <-receivedMessage:
		if string(msg) != string(testMessage) {
			t.Errorf("got message %s, want %s", string(msg), string(testMessage))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	if fmt.Sprint(gotReceivedHeaders) != fmt.Sprint(receivedHeaders) {
		t.Errorf("got received headers %+v, want %+v", gotReceivedHeaders, receivedHeaders)
	}

	gotSentHeaders := stream.Headers()
	if fmt.Sprint(gotSentHeaders) != fmt.Sprint(sentHeaders) {
		t.Errorf("got sent headers %+v, want %+v", gotSentHeaders, sentHeaders)
	}
}

// TestHeadler_NoTraceCapability tests that Headler is not called when capabilities don't match
func TestHeadler_NoTraceCapability(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1, overlay1 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		FullNode:           true,
		EnableTraceHeaders: true, // Local supports
	}})

	s2, overlay2 := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{
		EnableTraceHeaders: false, // Remote doesn't support
	}})

	headlerCalled := make(chan struct{}, 1)
	handlerCalled := make(chan struct{})
	testMessage := []byte("test-message-headler-no-caps")
	receivedMessage := make(chan []byte, 1)

	if err := s1.AddProtocol(p2p.ProtocolSpec{
		Name:    testProtocolName,
		Version: testProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name: testStreamName,
				Handler: func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
					defer close(handlerCalled)
					// Read test message from stream
					buf := make([]byte, len(testMessage))
					_, err := stream.Read(buf)
					if err != nil {
						t.Errorf("failed to read from stream: %v", err)
					}
					receivedMessage <- buf

					// Verify no headers were received
					headers := stream.Headers()
					if len(headers) != 0 {
						t.Errorf("expected no headers due to capability mismatch, got %+v", headers)
					}
					return nil
				},
				Headler: func(headers p2p.Headers, address swarm.Address) p2p.Headers {
					select {
					case headlerCalled <- struct{}{}:
					default:
					}
					t.Error("Headler should not be called when capabilities don't match")
					return nil
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

	// Wait for handshake to complete and capabilities to be registered
	expectPeers(t, s2, overlay1)
	expectPeersEventually(t, s1, overlay2)

	// Provide headers but they should be ignored
	providedHeaders := p2p.Headers{
		"should-be-ignored": []byte("ignored-value"),
	}

	stream, err := s2.NewStream(ctx, overlay1, providedHeaders, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	// Send test message to verify stream is working
	_, err = stream.Write(testMessage)
	if err != nil {
		t.Errorf("failed to write to stream: %v", err)
	}

	select {
	case <-handlerCalled:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Verify message was received
	select {
	case msg := <-receivedMessage:
		if string(msg) != string(testMessage) {
			t.Errorf("got message %s, want %s", string(msg), string(testMessage))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	// Verify Headler was NOT called
	select {
	case <-headlerCalled:
		t.Error("Headler should not have been called when capabilities don't match")
	case <-time.After(1 * time.Second):
		// Expected - Headler should not be called
	}

	// Verify stream has no headers
	headers := stream.Headers()
	if len(headers) != 0 {
		t.Errorf("expected no headers when capabilities don't match, got %+v", headers)
	}
}

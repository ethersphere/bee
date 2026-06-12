// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pubsub_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pubsub"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// pipeStream is a p2p.Stream backed by an io.Pipe pair for bidirectional use.
type pipeStream struct {
	pr      *io.PipeReader
	pw      *io.PipeWriter
	headers p2p.Headers
}

func (p *pipeStream) Read(b []byte) (int, error)   { return p.pr.Read(b) }
func (p *pipeStream) Write(b []byte) (int, error)  { return p.pw.Write(b) }
func (p *pipeStream) Close() error                 { p.pr.Close(); p.pw.Close(); return nil }
func (p *pipeStream) ResponseHeaders() p2p.Headers { return nil }
func (p *pipeStream) Headers() p2p.Headers         { return p.headers }
func (p *pipeStream) FullClose() error             { return p.Close() }
func (p *pipeStream) Reset() error {
	p.pr.CloseWithError(io.ErrUnexpectedEOF)
	p.pw.CloseWithError(io.ErrUnexpectedEOF)
	return nil
}

// newService creates a pubsub.Service with a nil P2P backend.
// This is valid for tests that never reach any P2P method call.
func newService(t *testing.T, brokerMode bool) *pubsub.Service {
	t.Helper()
	return pubsub.New(nil, log.Noop, brokerMode)
}

func TestBrokerHandler_Disabled(t *testing.T) {
	t.Parallel()

	svc := newService(t, false)
	handler := svc.Protocol().StreamSpecs[0].Handler

	var topicAddr [32]byte
	headers := p2p.Headers{
		pubsub.HeaderTopicAddress: topicAddr[:],
		pubsub.HeaderMode:         {byte(pubsub.ModeGSOCEphemeral)},
		pubsub.HeaderReadWrite:    {0},
	}
	stream := newReaderStream(nil, headers)
	peer := p2p.Peer{Address: swarm.NewAddress(make([]byte, 32))}

	err := handler(context.Background(), peer, stream)
	if !errors.Is(err, pubsub.ErrBrokerDisabled) {
		t.Fatalf("expected ErrBrokerDisabled, got %v", err)
	}
}

func TestBrokerHandler_MissingTopicAddress(t *testing.T) {
	t.Parallel()

	svc := newService(t, true)
	handler := svc.Protocol().StreamSpecs[0].Handler

	headers := p2p.Headers{
		pubsub.HeaderMode: {byte(pubsub.ModeGSOCEphemeral)},
	}
	stream := newReaderStream(nil, headers)
	peer := p2p.Peer{Address: swarm.NewAddress(make([]byte, 32))}

	err := handler(context.Background(), peer, stream)
	if !errors.Is(err, pubsub.ErrWrongHeaders) {
		t.Fatalf("expected ErrWrongHeaders, got %v", err)
	}
}

func TestBrokerHandler_MissingMode(t *testing.T) {
	t.Parallel()

	svc := newService(t, true)
	handler := svc.Protocol().StreamSpecs[0].Handler

	var topicAddr [32]byte
	headers := p2p.Headers{
		pubsub.HeaderTopicAddress: topicAddr[:],
	}
	stream := newReaderStream(nil, headers)
	peer := p2p.Peer{Address: swarm.NewAddress(make([]byte, 32))}

	err := handler(context.Background(), peer, stream)
	if !errors.Is(err, pubsub.ErrWrongHeaders) {
		t.Fatalf("expected ErrWrongHeaders, got %v", err)
	}
}

func TestBrokerHandler_UnknownMode(t *testing.T) {
	t.Parallel()

	svc := newService(t, true)
	handler := svc.Protocol().StreamSpecs[0].Handler

	var topicAddr [32]byte
	headers := p2p.Headers{
		pubsub.HeaderTopicAddress: topicAddr[:],
		pubsub.HeaderMode:         {0xff},
	}
	stream := newReaderStream(nil, headers)
	peer := p2p.Peer{Address: swarm.NewAddress(make([]byte, 32))}

	err := handler(context.Background(), peer, stream)
	if err == nil {
		t.Fatal("expected error for unknown mode, got nil")
	}
	if errors.Is(err, pubsub.ErrBrokerDisabled) || errors.Is(err, pubsub.ErrWrongHeaders) {
		t.Fatalf("unexpected sentinel error for unknown mode: %v", err)
	}
}

func TestService_Topics_Empty(t *testing.T) {
	t.Parallel()

	svc := newService(t, true)
	if topics := svc.Topics(); len(topics) != 0 {
		t.Fatalf("expected empty topics, got %d", len(topics))
	}
}

func TestService_Topics_BrokerRole(t *testing.T) {
	t.Parallel()

	svc := newService(t, true)
	handler := svc.Protocol().StreamSpecs[0].Handler

	tc := newSocTestCtx(t)
	headers := p2p.Headers{
		pubsub.HeaderTopicAddress: tc.topicAddr[:],
		pubsub.HeaderMode:         {byte(pubsub.ModeGSOCEphemeral)},
		pubsub.HeaderReadWrite:    {0}, // subscriber on broker side
	}

	pr, pw := io.Pipe()
	stream := &pipeStream{pr: pr, pw: pw, headers: headers}
	peer := p2p.Peer{Address: swarm.NewAddress(make([]byte, 32))}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = stream.Close()
	})

	handlerErrCh := make(chan error, 1)
	go func() {
		handlerErrCh <- handler(ctx, peer, stream)
	}()

	err := spinlock.Wait(time.Second, func() bool {
		for _, topic := range svc.Topics() {
			if topic.Role == "broker" {
				return true
			}
		}
		return false
	})
	if err != nil {
		t.Fatal("timed out waiting for broker topic to appear")
	}

	cancel()
	_ = stream.Close()
	<-handlerErrCh
}

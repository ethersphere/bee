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
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/tracing"
)

func TestTracing(t *testing.T) {
	tracer1, closer1, err := tracing.NewTracer(&tracing.Options{
		Enabled:     true,
		ServiceName: "bee-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closer1.Close()

	tracer2, closer2, err := tracing.NewTracer(&tracing.Options{
		Enabled:     true,
		ServiceName: "bee-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closer2.Close()

	s1, overlay1 := newService(t, 1, libp2p.Options{})

	s2, _ := newService(t, 1, libp2p.Options{})

	var handledTracingSpan string
	handled := make(chan struct{})
	if err := s1.AddProtocol(newTestProtocol(func(ctx context.Context, _ p2p.Peer, _ p2p.Stream) error {

		span, _, _ := tracer1.StartSpanFromContext(ctx, "test-p2p-handler", nil)
		defer span.Finish()

		handledTracingSpan = fmt.Sprint(span.Context())
		close(handled)
		return nil
	})); err != nil {
		t.Fatal(err)
	}

	addr := serviceUnderlayAddress(t, s1)

	connectContext, connectCancel := context.WithCancel(context.Background())
	defer connectCancel()

	if _, err := s2.Connect(connectContext, addr, false); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	span, _, ctx := tracer2.StartSpanFromContext(ctx, "test-p2p-client", nil)
	defer span.Finish()

	if fmt.Sprint(span.Context()) == "" {
		t.Error("not tracing span context to send")
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

	if handledTracingSpan == "" {
		t.Error("got not tracing span context in handler")
	}
}

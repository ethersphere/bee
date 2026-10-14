// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var streamNames = []string{
	rangesync.StatusStreamName,
	rangesync.ReconcileStreamName,
	rangesync.FetchStreamName,
}

// resetRecordingStream counts the Reset calls a handler makes on its stream.
type resetRecordingStream struct {
	p2p.Stream
	resets *atomic.Int32
}

func (s *resetRecordingStream) Reset() error {
	s.resets.Add(1)
	return s.Stream.Reset()
}

func TestProtocol(t *testing.T) {
	t.Parallel()

	s := rangesync.New(streamtest.New(), log.Noop)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})

	spec := s.Protocol()
	if spec.Name != "rangesync" {
		t.Errorf("name: got %q", spec.Name)
	}
	if spec.Version != "1.0.0" {
		t.Errorf("version: got %q", spec.Version)
	}
	if spec.Name != rangesync.ProtocolName || spec.Version != rangesync.ProtocolVersion {
		t.Errorf("spec differs from the exported constants")
	}
	if len(spec.StreamSpecs) != len(streamNames) {
		t.Fatalf("got %d streams, want %d", len(spec.StreamSpecs), len(streamNames))
	}
	for i, want := range []string{"status", "reconcile", "fetch"} {
		ss := spec.StreamSpecs[i]
		if ss.Name != want || ss.Name != streamNames[i] {
			t.Errorf("stream %d: got %q, want %q", i, ss.Name, want)
		}
		if ss.Handler == nil {
			t.Errorf("stream %q: no handler", ss.Name)
		}
	}
	if spec.ConnectIn != nil || spec.ConnectOut != nil || spec.DisconnectIn != nil || spec.DisconnectOut != nil {
		t.Error("unexpected connect/disconnect hooks")
	}
}

func TestHandlersNotImplemented(t *testing.T) {
	t.Parallel()

	for _, name := range streamNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := rangesync.New(streamtest.New(), log.Noop)
			var resets atomic.Int32
			recordReset := func(h p2p.HandlerFunc) p2p.HandlerFunc {
				return func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
					return h(ctx, p, &resetRecordingStream{Stream: s, resets: &resets})
				}
			}
			recorder := streamtest.New(
				streamtest.WithProtocols(server.Protocol()),
				streamtest.WithMiddlewares(recordReset),
			)
			peer := swarm.RandAddress(t)

			stream, err := recorder.NewStream(context.Background(), peer, nil, rangesync.ProtocolName, rangesync.ProtocolVersion, name)
			if err != nil {
				t.Fatal(err)
			}

			records, err := recorder.Records(peer, rangesync.ProtocolName, rangesync.ProtocolVersion, name)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 {
				t.Fatalf("got %d records, want 1", len(records))
			}
			if err := records[0].Err(); !errors.Is(err, rangesync.ErrNotImplemented) {
				t.Fatalf("handler error: got %v, want %v", err, rangesync.ErrNotImplemented)
			}

			if got := resets.Load(); got != 1 {
				t.Fatalf("handler reset the stream %d times, want 1", got)
			}
			// the stream is gone for the requester (streamtest maps Reset to a
			// full close, so this alone cannot tell Reset from Close)
			if _, err := stream.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
				t.Fatalf("read: got %v, want %v", err, io.EOF)
			}
		})
	}
}

func TestUnknownStream(t *testing.T) {
	t.Parallel()

	server := rangesync.New(streamtest.New(), log.Noop)
	recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

	_, err := recorder.NewStream(context.Background(), swarm.RandAddress(t), nil, rangesync.ProtocolName, rangesync.ProtocolVersion, "pullsync")
	if !errors.Is(err, streamtest.ErrStreamNotSupported) {
		t.Fatalf("got %v, want %v", err, streamtest.ErrStreamNotSupported)
	}
}

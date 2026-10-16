// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// newService returns a service over an empty mock reserve.
func newService(t *testing.T, streamer p2p.StreamerDisconnecter) *rangesync.Service {
	t.Helper()
	s := rangesync.New(streamer, mockstorer.NewReserve(mockstorer.WithAddrIndex()), swarm.RandAddress(t), nil, nil, nil, log.Noop, rangesync.Options{})
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

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

	s := newService(t, streamtest.NewRecorderDisconnecter(streamtest.New()))

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
	if spec.ConnectIn != nil || spec.ConnectOut != nil {
		t.Error("unexpected connect hooks")
	}
	if spec.DisconnectIn == nil || spec.DisconnectOut == nil {
		t.Error("missing disconnect hooks")
	}
}

func TestUnknownStream(t *testing.T) {
	t.Parallel()

	server := newService(t, streamtest.NewRecorderDisconnecter(streamtest.New()))
	recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

	_, err := recorder.NewStream(context.Background(), swarm.RandAddress(t), nil, rangesync.ProtocolName, rangesync.ProtocolVersion, "pullsync")
	if !errors.Is(err, streamtest.ErrStreamNotSupported) {
		t.Fatalf("got %v, want %v", err, streamtest.ErrStreamNotSupported)
	}
}

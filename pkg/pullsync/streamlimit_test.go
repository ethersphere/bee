// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	mock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestPeerStreamLimiter(t *testing.T) {
	t.Parallel()

	const limit = 3
	var (
		l     = pullsync.NewPeerStreamLimiter(limit)
		peer  = swarm.RandAddress(t)
		other = swarm.RandAddress(t)
	)

	releases := make([]func(), 0, limit)
	for range limit {
		release, ok := l.Acquire(peer)
		if !ok {
			t.Fatal("expected slot to be acquired")
		}
		releases = append(releases, release)
	}

	if _, ok := l.Acquire(peer); ok {
		t.Fatal("expected limit to be reached")
	}
	if _, ok := l.Acquire(other); !ok {
		t.Fatal("expected other peer to have its own pool")
	}

	releases[0]()
	if _, ok := l.Acquire(peer); !ok {
		t.Fatal("expected slot after release")
	}

	l.Clear(peer)
	if _, ok := l.Acquire(peer); !ok {
		t.Fatal("expected fresh pool after clear")
	}
	// releasing slots of a cleared pool must not block
	releases[1]()
	releases[2]()
}

func TestHandler_StreamLimitExceeded(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		fill    func(*pullsync.Syncer, swarm.Address)
		handler func(*pullsync.Syncer) p2p.HandlerFunc
	}{
		{
			name:    "sync",
			fill:    (*pullsync.Syncer).FillSyncStreams,
			handler: func(s *pullsync.Syncer) p2p.HandlerFunc { return s.Handler },
		},
		{
			name:    "cursors",
			fill:    (*pullsync.Syncer).FillCursorStreams,
			handler: func(s *pullsync.Syncer) p2p.HandlerFunc { return s.CursorHandler },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ps, _ := newPullSync(t, nil, 0, mock.WithCursors([]uint64{1}, 1))
			peer := swarm.RandAddress(t)
			tc.fill(ps, peer)

			stream := &resetStream{}
			err := tc.handler(ps)(context.Background(), p2p.Peer{Address: peer}, stream)

			var de *p2p.DisconnectError
			if !errors.As(err, &de) {
				t.Fatalf("got error %v, want disconnect error", err)
			}
			if !errors.Is(err, pullsync.ErrStreamLimitExceeded) {
				t.Fatalf("got error %v, want %v", err, pullsync.ErrStreamLimitExceeded)
			}
			if !stream.reset {
				t.Fatal("expected stream to be reset")
			}
			if got := ps.StreamLimitExceededCount(); got != 1 {
				t.Fatalf("got metric %v, want 1", got)
			}
		})
	}
}

type resetStream struct {
	p2p.Stream
	reset bool
}

func (s *resetStream) Reset() error {
	s.reset = true
	return nil
}

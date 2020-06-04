// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"io"
	"math"
	"sync"

	"github.com/ethersphere/bee/pkg/pullsync"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ pullsync.Interface = (*PullSyncMock)(nil)

func WithCursors(v []uint64) Option {
	return optionFunc(func(p *PullSyncMock) {
		p.cursors = v
	})
}

// WithAutoReply means that the pull syncer will automatically reply
// to incoming range requests with a top = from+limit.
// This is in order to force the requester to request a subsequent range.
func WithAutoReply() Option {
	return optionFunc(func(p *PullSyncMock) {
		p.autoReply = true
	})
}

// WithLiveSyncBlock makes the protocol mock block on incoming live
// sync requests (identified by the math.MaxUint64 `to` field).
func WithLiveSyncBlock() Option {
	return optionFunc(func(p *PullSyncMock) {
		p.blockLiveSync = true
	})
}

func WithLiveSyncReplies(r ...uint64) Option {
	return optionFunc(func(p *PullSyncMock) {
		p.liveSyncReplies = r
	})
}

const limit = 50

type SyncCall struct {
	Peer     swarm.Address
	Bin      uint8
	From, To uint64
	Live     bool
}

type PullSyncMock struct {
	mtx             sync.Mutex
	syncCalls       []SyncCall
	cursors         []uint64
	getCursorsPeers []swarm.Address
	autoReply       bool
	blockLiveSync   bool
	liveSyncReplies []uint64
	liveSyncCalls   int

	quit chan struct{}
}

func NewPullSync(opts ...Option) *PullSyncMock {
	s := &PullSyncMock{
		quit: make(chan struct{}),
	}
	for _, v := range opts {
		v.apply(s)
	}
	return s
}

func (p *PullSyncMock) SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from uint64, to uint64) (topmost uint64, err error) {
	isLive := to == math.MaxUint64

	call := SyncCall{
		Peer: peer,
		Bin:  bin,
		From: from,
		To:   to,
		Live: isLive,
	}
	p.mtx.Lock()
	p.syncCalls = append(p.syncCalls, call)
	if isLive && p.blockLiveSync {
		// don't response, wait for context cancel
		p.mtx.Unlock()
		<-p.quit
		return 0, io.EOF
	}
	if isLive && len(p.liveSyncReplies) > 0 {
		if p.liveSyncCalls >= len(p.liveSyncReplies) {
			p.mtx.Unlock()
			<-p.quit
			return
		}
		v := p.liveSyncReplies[p.liveSyncCalls]
		p.liveSyncCalls++
		p.mtx.Unlock()
		return v, nil
	}
	p.mtx.Unlock()

	if p.autoReply {
		t := from + limit - 1
		// floor to the cursor
		if t > to {
			t = to
		}
		return t, nil
	}
	return to, nil
}

func (p *PullSyncMock) GetCursors(_ context.Context, peer swarm.Address) ([]uint64, error) {
	p.getCursorsPeers = append(p.getCursorsPeers, peer)
	return p.cursors, nil
}

func (p *PullSyncMock) SyncCalls(peer swarm.Address) (res []SyncCall) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, v := range p.syncCalls {
		if v.Peer.Equal(peer) && !v.Live {
			res = append(res, v)
		}
	}
	return res
}

func (p *PullSyncMock) LiveSyncCalls(peer swarm.Address) (res []SyncCall) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, v := range p.syncCalls {
		if v.Peer.Equal(peer) && v.Live {
			res = append(res, v)
		}
	}
	return res
}

func (p *PullSyncMock) CursorsCalls(peer swarm.Address) bool {
	for _, v := range p.getCursorsPeers {
		if v.Equal(peer) {
			return true
		}
	}
	return false
}

func (p *PullSyncMock) Close() error {
	close(p.quit)
	return nil
}

type Option interface {
	apply(*PullSyncMock)
}
type optionFunc func(*PullSyncMock)

func (f optionFunc) apply(r *PullSyncMock) { f(r) }

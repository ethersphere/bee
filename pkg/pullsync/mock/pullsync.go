// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
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

func WithLateSyncReply(r ...SyncReply) Option {
	return optionFunc(func(p *PullSyncMock) {
		p.lateReply = true
		p.lateSyncReplies = r
	})
}

const limit = 50

type SyncCall struct {
	Peer     swarm.Address
	Bin      uint8
	From, To uint64
	Live     bool
}

type SyncReply struct {
	bin     uint8
	from    uint64
	topmost uint64
	block   bool
}

func NewReply(bin uint8, from, top uint64, block bool) SyncReply {
	return SyncReply{
		bin:     bin,
		from:    from,
		topmost: top,
		block:   block,
	}
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

	lateReply       bool
	lateCond        *sync.Cond
	lateChange      bool
	lateSyncReplies []SyncReply

	quit chan struct{}
}

func NewPullSync(opts ...Option) *PullSyncMock {
	s := &PullSyncMock{
		lateCond: sync.NewCond(new(sync.Mutex)),
		quit:     make(chan struct{}),
	}
	for _, v := range opts {
		v.apply(s)
	}
	return s
}

func (p *PullSyncMock) SyncInterval(ctx context.Context, peer swarm.Address, bin uint8, from, to uint64) (topmost uint64, ruid uint32, err error) {
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
	p.mtx.Unlock()

	if isLive && p.lateReply {
		p.lateCond.L.Lock()
		for !p.lateChange {
			p.lateCond.Wait()
		}
		p.lateCond.L.Unlock()

		select {
		case <-p.quit:
			return 0, 1, context.Canceled
		case <-ctx.Done():
			return 0, 1, ctx.Err()
		default:
		}

		found := false
		var sr SyncReply
		p.mtx.Lock()
		for i, v := range p.lateSyncReplies {
			if v.bin == bin && v.from == from {
				sr = v
				found = true
				p.lateSyncReplies = append(p.lateSyncReplies[:i], p.lateSyncReplies[i+1:]...)
			}
		}
		p.mtx.Unlock()
		if found {
			if sr.block {
				select {
				case <-p.quit:
					return 0, 1, context.Canceled
				case <-ctx.Done():
					return 0, 1, ctx.Err()
				}
			}
			return sr.topmost, 0, nil
		}
		panic("not found")
	}

	if isLive && p.blockLiveSync {
		// don't respond, wait for quit
		<-p.quit
		return 0, 1, context.Canceled
	}

	if isLive && len(p.liveSyncReplies) > 0 {
		if p.liveSyncCalls >= len(p.liveSyncReplies) {
			<-p.quit
			// when shutting down, onthe puller side we cancel the context going into the pullsync protocol request
			// this results in SyncInterval returning with a context cancelled error
			return 0, 0, context.Canceled
		}
		p.mtx.Lock()
		v := p.liveSyncReplies[p.liveSyncCalls]
		p.liveSyncCalls++
		p.mtx.Unlock()
		return v, 1, nil
	}

	if p.autoReply {
		t := from + limit - 1
		// floor to the cursor
		if t > to {
			t = to
		}
		return t, 1, nil
	}
	return to, 1, nil
}

func (p *PullSyncMock) GetCursors(_ context.Context, peer swarm.Address) ([]uint64, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
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

func (p *PullSyncMock) CancelRuid(ctx context.Context, peer swarm.Address, ruid uint32) error {
	return nil
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
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, v := range p.getCursorsPeers {
		if v.Equal(peer) {
			return true
		}
	}
	return false
}

func (p *PullSyncMock) TriggerChange() {
	p.lateCond.L.Lock()
	p.lateChange = true
	p.lateCond.L.Unlock()
	p.lateCond.Broadcast()
}

func (p *PullSyncMock) Close() error {
	close(p.quit)
	p.lateCond.L.Lock()
	p.lateChange = true
	p.lateCond.L.Unlock()
	p.lateCond.Broadcast()
	return nil
}

type Option interface {
	apply(*PullSyncMock)
}
type optionFunc func(*PullSyncMock)

func (f optionFunc) apply(r *PullSyncMock) { f(r) }

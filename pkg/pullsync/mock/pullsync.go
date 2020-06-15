// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"fmt"
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

// the list of SyncReply is not ordered
func WithExactLiveSyncReplies(r ...SyncReply) Option {
	return optionFunc(func(p *PullSyncMock) {
		p.liveSyncExactReplies = r
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
	mtx                  sync.Mutex
	syncCalls            []SyncCall
	cursors              []uint64
	getCursorsPeers      []swarm.Address
	autoReply            bool
	blockLiveSync        bool
	liveSyncReplies      []uint64
	liveSyncCalls        int
	liveSyncExactReplies []SyncReply

	lateReply bool
	lateCond  *sync.Cond
	//lateReads       int
	lateTop         uint64
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

	if isLive && p.lateReply {
		p.mtx.Unlock()
		p.lateCond.L.Lock()
		defer p.lateCond.L.Unlock()

		for p.lateTop == 0 {
			select {
			case <-p.quit:
				return 0, context.Canceled
			case <-ctx.Done():
				return 0, ctx.Err()
			default:
			}
			p.lateCond.Wait()
		}

		select {
		case <-p.quit:
			return 0, context.Canceled
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		for _, v := range p.lateSyncReplies {
			if v.bin == bin && v.from == from {
				if v.block {
					select {
					case <-p.quit:
						return 0, context.Canceled
					case <-ctx.Done():
						return 0, ctx.Err()
					}
				}
				return v.topmost, nil
			}
		}
		fmt.Println("did not find element for bin", bin)
		return 0, context.Canceled
		//p.lateReads--
		//top := p.lateTop
		//if p.lateReads == 0 {
		//p.lateTop = 0
		//}
		//return top, nil
	}

	if isLive && p.blockLiveSync {
		// don't response, wait for context cancel
		p.mtx.Unlock()
		<-p.quit
		return 0, io.EOF
	}

	if isLive && len(p.liveSyncExactReplies) > 0 {
		var (
			sr    SyncReply
			found bool
		)
		for i, v := range p.liveSyncExactReplies {
			if v.bin == bin && v.from == from {
				sr = v
				found = true
				p.liveSyncExactReplies = append(p.liveSyncExactReplies[:i], p.liveSyncExactReplies[i+1:]...)
			}
			if found {
				break
			}
		}
		p.mtx.Unlock()
		if !found {
			panic("not found")
		}
		if sr.block {
			<-p.quit
			return
		}
		return sr.topmost, nil
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

func (p *PullSyncMock) Broadcast() {
	p.lateCond.Broadcast()
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

func (p *PullSyncMock) TriggerTopmost(t uint64) {
	p.lateCond.L.Lock()
	defer p.lateCond.L.Unlock()
	p.lateTop = t
	p.lateCond.Broadcast()
}

func (p *PullSyncMock) Close() error {
	close(p.quit)
	p.lateCond.L.Lock()
	defer p.lateCond.L.Unlock()
	p.lateTop = 1
	p.lateCond.Broadcast()
	return nil
}

type Option interface {
	apply(*PullSyncMock)
}
type optionFunc func(*PullSyncMock)

func (f optionFunc) apply(r *PullSyncMock) { f(r) }

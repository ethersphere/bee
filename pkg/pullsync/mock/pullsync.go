// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var _ pullsync.Interface = (*PullSyncMock)(nil)

func WithSyncError(err error) Option {
	return optionFunc(func(p *PullSyncMock) {
		p.syncErr = err
	})
}

func WithCursors(v []uint64, e uint64) Option {
	return optionFunc(func(p *PullSyncMock) {
		p.cursors = v
		p.epoch = e
	})
}

func WithReplies(replies ...SyncReply) Option {
	return optionFunc(func(p *PullSyncMock) {
		for _, r := range replies {
			p.replies[toID(r.Peer, r.Bin, r.Start)] = append(p.replies[toID(r.Peer, r.Bin, r.Start)], r)
		}
	})
}

func toID(a swarm.Address, bin uint8, start uint64) string {
	return fmt.Sprintf("%s-%d-%d", a, bin, start)
}

type SyncReply struct {
	Peer    swarm.Address
	Bin     uint8
	Start   uint64
	Topmost uint64
	Count   int
}

type PullSyncMock struct {
	mtx             sync.Mutex
	syncCalls       []SyncReply
	syncErr         error
	cursors         []uint64
	epoch           uint64
	getCursorsPeers []swarm.Address
	replies         map[string][]SyncReply

	quit chan struct{}
}

func NewPullSync(opts ...Option) *PullSyncMock {
	s := &PullSyncMock{
		quit:    make(chan struct{}),
		replies: make(map[string][]SyncReply),
	}
	for _, v := range opts {
		v.apply(s)
	}
	return s
}

func (p *PullSyncMock) Sync(ctx context.Context, peer swarm.Address, bin uint8, start uint64) (topmost uint64, count int, err error) {

	p.mtx.Lock()

	id := toID(peer, bin, start)
	replies := p.replies[id]

	if len(replies) > 0 {
		reply := replies[0]
		p.replies[id] = p.replies[id][1:]
		p.syncCalls = append(p.syncCalls, reply)
		p.mtx.Unlock()
		return reply.Topmost, reply.Count, p.syncErr
	}
	p.mtx.Unlock()
	<-ctx.Done()
	return 0, 0, ctx.Err()
}

func (p *PullSyncMock) GetCursors(_ context.Context, peer swarm.Address) ([]uint64, uint64, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.getCursorsPeers = append(p.getCursorsPeers, peer)
	return p.cursors, p.epoch, nil
}

func (p *PullSyncMock) ResetCalls(peer swarm.Address) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.syncCalls = nil
}

func (p *PullSyncMock) SyncCalls(peer swarm.Address) (res []SyncReply) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, v := range p.syncCalls {
		if v.Peer.Equal(peer) {
			res = append(res, v)
		}
	}
	return res
}

func (p *PullSyncMock) CursorsCalls(peer swarm.Address) bool {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return swarm.ContainsAddress(p.getCursorsPeers, peer)
}

func (p *PullSyncMock) SetEpoch(epoch uint64) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.epoch = epoch
}

type Option interface {
	apply(*PullSyncMock)
}
type optionFunc func(*PullSyncMock)

func (f optionFunc) apply(r *PullSyncMock) { f(r) }

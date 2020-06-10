// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/intervalstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pullsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

const bins = 5

type Options struct {
	StateStore storage.StateStorer
	Topology   topology.Driver
	PullSync   pullsync.Interface
	Logger     logging.Logger
}

type Puller struct {
	mtx         sync.Mutex
	topology    topology.Driver
	statestore  storage.StateStorer
	intervalMtx sync.Mutex
	syncer      pullsync.Interface
	logger      logging.Logger

	syncPeers    []map[string]*syncPeer // index is bin, map key is peer address
	syncPeersMtx sync.Mutex

	cursors    map[string][]uint64
	cursorsMtx sync.Mutex

	peersC chan struct{}
	quit   chan struct{}
}

func New(o Options) *Puller {
	p := &Puller{
		statestore: o.StateStore,
		topology:   o.Topology,
		syncer:     o.PullSync,
		logger:     o.Logger,

		cursors: make(map[string][]uint64),

		peersC:    make(chan struct{}, 1),
		syncPeers: make([]map[string]*syncPeer, bins),
		quit:      make(chan struct{}),
	}

	for i := 0; i < bins; i++ {
		p.syncPeers[i] = make(map[string]*syncPeer)
	}

	go p.pollTopology()
	go p.manage()
	return p
}

func (p *Puller) pollTopology() {
	c, unsubscribe := p.topology.SubscribePeersChange()
	defer unsubscribe()

	// this is to dampen the amount of peer change notification we get
	//todo: this should become into some falling-edge filter, since this is till rising edge
	// ie. reset a timer every time we get a notification on the channel, then once a certain timeout
	// passes we trigger the peersC channel
	for {
		select {
		case <-c:
			time.Sleep(10 * time.Millisecond)
			p.peersC <- struct{}{}
		case <-p.quit:
			return
		}
	}
}

type peer struct {
	addr swarm.Address
	po   uint8
}

func (p *Puller) manage() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		select {
		case <-p.peersC:
			// get all peers from kademlia
			// iterate on entire bin at once (get all peers first)
			// check how many intervals we synced with all of them
			// pick the one with the most
			// sync with that one

			// if we're already syncing with this peer, make sure
			// that we're syncing the correct bins according to depth
			depth := p.topology.NeighborhoodDepth()

			// we defer the actual start of syncing to get out of the iterator first
			var peersToSync []peer

			p.syncPeersMtx.Lock()
			defer p.syncPeersMtx.Unlock()

			err := p.topology.EachPeerRev(func(peerAddr swarm.Address, po uint8) (stop, jumpToNext bool, err error) {
				bp := p.syncPeers[po]
				switch {
				case po < depth:
					// outside of depth, sync bin only
					// and only with one peer (or some other const)

					// if we are already syncing with someone then move on to next
					if len(bp) > 0 {
						return false, true, nil // skip to next bin
					}

					if _, ok := bp[peerAddr.String()]; !ok {
						bp[peerAddr.String()] = newSyncPeer()
						peerEntry := peer{addr: peerAddr, po: po}
						peersToSync = append(peersToSync, peerEntry)
						// not yet syncing, start
					}
				case po >= depth:
					//	within depth, sync everything >= depth
					if _, ok := bp[peerAddr.String()]; !ok {
						// we're not syncing with this peer yet, start doing so
						bp[peerAddr.String()] = newSyncPeer()
						peerEntry := peer{addr: peerAddr, po: po}
						peersToSync = append(peersToSync, peerEntry)
					} else {
						// already syncing, recalc
					}
				}

				return false, false, nil
			})

			if err != nil {
				panic(err)
			}

			for _, v := range peersToSync {
				p.syncPeer(ctx, v.addr, v.po)
			}

		case <-p.quit:
			return
		}
	}
}

func (p *Puller) syncPeer(ctx context.Context, peer swarm.Address, po uint8) {
	p.cursorsMtx.Lock()
	c, ok := p.cursors[peer.String()]
	p.cursorsMtx.Unlock()

	if !ok {
		cursors, err := p.syncer.GetCursors(ctx, peer)
		if err != nil {
			// remove from syncing peers list, trigger channel to find some other peer
			// maybe blacklist for some time
		}
		p.cursorsMtx.Lock()
		p.cursors[peer.String()] = cursors
		p.cursorsMtx.Unlock()
		c = cursors
	}
	syncCtx := p.syncPeers[po][peer.String()]

	syncCtx.Lock()
	defer syncCtx.Unlock()

	d := p.topology.NeighborhoodDepth()

	// peer outside depth?
	if po < d {
		cur, bin := c[po], po
		// start just one bin for historical and live
		hbinCtx, hcancel := context.WithCancel(ctx)
		syncCtx.histCancelFuncs[po] = hcancel

		go p.histSyncWorker(hbinCtx, peer, uint8(bin), cur)
		lbinCtx, lcancel := context.WithCancel(ctx)
		syncCtx.liveCancelFuncs[po] = lcancel
		go p.liveSyncWorker(lbinCtx, peer, uint8(bin), cur)

		return
	}

	// start historical
	for bin, cur := range c {
		if bin == 0 || uint8(bin) < d || cur == 0 {
			continue
		}
		binCtx, cancel := context.WithCancel(ctx)
		syncCtx.histCancelFuncs[po] = cancel
		go p.histSyncWorker(binCtx, peer, uint8(bin), cur)
	}

	// start live
	for bin, cur := range c {
		if bin == 0 || uint8(bin) < d {
			continue
		}
		binCtx, cancel := context.WithCancel(ctx)
		syncCtx.liveCancelFuncs[po] = cancel
		go p.liveSyncWorker(binCtx, peer, uint8(bin), cur)
	}
}

func (p *Puller) histSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	p.logger.Debugf("histSyncWorker starting, peer %s bin %d cursor %d", peer, bin, cur)
	for {
		select {
		case <-p.quit:
			return
		case <-ctx.Done():
			return
		default:
		}

		s, _, _, err := p.nextPeerInterval(peer, bin)
		if err != nil {
			p.logger.Debugf("histSyncWorker nextPeerInterval: %v", err)
			// wait and retry? this is a local error
			// maybe just quit the peer entirely.
			// not sure how to do this
			<-time.After(30 * time.Second)
			continue
		}
		p.logger.Debugf("histSyncWorker peer %s bin %d next interval %d cursor %d", peer, bin, s, cur)
		if s > cur {
			//syncCtx := p.syncPeers[bin][peer.String()]
			//syncCtx.Lock()
			//f := syncCtx.histCancelFuncs[bin]
			//f()
			//delete(syncCtx.histCancelFuncs, bin)
			//syncCtx.Unlock()
			p.logger.Debugf("histSyncWorker finished syncing bin %d, cursor %d", bin, cur)
			return
		}
		start := time.Now()
		top, err := p.syncer.SyncInterval(ctx, peer, uint8(bin), s, cur)
		if err != nil {
			p.logger.Debugf("histSyncWorker error syncing interval. peer %s, bin %d, cursor %d, err %v", peer.String(), bin, cur, err)

		}
		took := time.Since(start)
		p.logger.Tracef("histSyncWorker peer %s bin %d synced interval from %d to %d. took %s", peer, bin, s, top, took)
		p.addPeerInterval(peer, bin, s, top)
	}
}

func (p *Puller) liveSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	p.logger.Debugf("liveSyncWorker starting, peer %s bin %d cursor %d", peer, bin, cur)
	from := cur + 1
	for {
		select {
		case <-p.quit:
			return
		case <-ctx.Done():
			return
		default:
		}
		p.logger.Tracef("liveSyncWorker peer %s syncing bin %d from %d", peer, bin, from)
		top, err := p.syncer.SyncInterval(ctx, peer, bin, from, math.MaxUint64)
		if err != nil {
			return
		}
		p.logger.Tracef("liveSyncWorker peer %s synced bin %d from %d to %d", peer, bin, from, top)
		p.addPeerInterval(peer, bin, from, top)
		from = top + 1
	}
}

func (p *Puller) Close() error {
	close(p.quit)
	return nil
}

func (p *Puller) addPeerInterval(peer swarm.Address, bin uint8, start, end uint64) (err error) {
	p.intervalMtx.Lock()
	defer p.intervalMtx.Unlock()

	peerStreamKey := peerIntervalKey(peer, bin)
	i, err := p.getOrCreateInterval(peer, bin)
	if err != nil {
		return err
	}
	i.Add(start, end)
	return p.statestore.Put(peerStreamKey, i)
}

func (p *Puller) nextPeerInterval(peer swarm.Address, bin uint8) (start, end uint64, empty bool, err error) {
	p.intervalMtx.Lock()
	defer p.intervalMtx.Unlock()

	i, err := p.getOrCreateInterval(peer, bin)
	if err != nil {
		return 0, 0, false, err
	}

	start, end, empty = i.Next(0)
	return start, end, empty, nil
}

func (p *Puller) getOrCreateInterval(peer swarm.Address, bin uint8) (*intervalstore.Intervals, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	// check that an interval entry exists
	key := peerIntervalKey(peer, bin)
	i := &intervalstore.Intervals{}
	err := p.statestore.Get(key, i)
	switch err {
	case nil:
	case storage.ErrNotFound:
		// key interval values are ALWAYS > 0
		i = intervalstore.NewIntervals(1)
		if err := p.statestore.Put(key, i); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("get peer interval: %w", err)
	}
	return i, nil
}

func peerIntervalKey(peer swarm.Address, bin uint8) string {
	k := fmt.Sprintf("%s|%d", peer.String(), bin)
	return k
}

type syncPeer struct {
	histCancelFuncs map[uint8]func() // slice of context cancel funcs for historical sync. index is bin
	liveCancelFuncs map[uint8]func() // slice of context cancel funcs for live sync. index is bin

	sync.Mutex
}

func newSyncPeer() *syncPeer {
	return &syncPeer{
		histCancelFuncs: make(map[uint8]func(), bins),
		liveCancelFuncs: make(map[uint8]func(), bins),
	}
}

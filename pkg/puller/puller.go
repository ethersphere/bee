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
	// todo: this should become into some falling-edge filter, since this is till rising edge
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
			var (
				peersToSync   []peer
				peersToRecalc []peer
			)
			p.syncPeersMtx.Lock()

			err := p.topology.EachPeerRev(func(peerAddr swarm.Address, po uint8) (stop, jumpToNext bool, err error) {
				bp := p.syncPeers[po]
				syncing := len(bp)
				switch {
				case po < depth:
					// outside of depth, sync bin only
					// and only with one peer (or some other const)

					// if we are already syncing with someone then move on to next
					// not yet syncing, start

					if _, ok := bp[peerAddr.String()]; !ok {
						if syncing < 1 {
							bp[peerAddr.String()] = newSyncPeer()
							peerEntry := peer{addr: peerAddr, po: po}
							peersToSync = append(peersToSync, peerEntry)
						}
					} else {
						// already syncing, recalc
						peerEntry := peer{addr: peerAddr, po: po}
						peersToRecalc = append(peersToRecalc, peerEntry)
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
						peerEntry := peer{addr: peerAddr, po: po}
						peersToRecalc = append(peersToRecalc, peerEntry)
					}
				}

				return false, false, nil
			})

			if err != nil {
				panic(err)
			}

			for _, v := range peersToSync {
				p.syncPeer(ctx, v.addr, v.po, depth)
			}

			for _, v := range peersToRecalc {
				p.recalcPeer(ctx, v.addr, v.po, depth)
			}
			p.syncPeersMtx.Unlock()

		case <-p.quit:
			return
		}
	}
}

func (p *Puller) recalcPeer(ctx context.Context, peer swarm.Address, po uint8, d uint8) {
	p.logger.Debugf("puller recalculating peer %s po %d depth %d", peer, po, d)
	syncCtx := p.syncPeers[po][peer.String()]

	syncCtx.Lock()
	defer syncCtx.Unlock()

	p.cursorsMtx.Lock()
	c := p.cursors[peer.String()]
	p.cursorsMtx.Unlock()

	if po >= d {
		// within depth
		var want, dontWant []uint8

		for i := d; i < bins; i++ {
			if i == 0 {
				continue
			}
			want = append(want, i)
		}
		for i := uint8(0); i < d; i++ {
			dontWant = append(dontWant, i)
		}

		for _, bin := range want {
			// question: do we want to have the separate cancel funcs per live/hist
			// for known whether syncing is running on that bin/stream? could be some race here
			if _, ok := syncCtx.binCancelFuncs[bin]; !ok {
				// if there's no bin cancel func it means there's no
				// sync running on this bin. start syncing both hist and live
				cur := c[bin]
				binCtx, cancel := context.WithCancel(ctx)
				syncCtx.binCancelFuncs[uint8(bin)] = cancel
				if cur > 0 {
					go p.histSyncWorker(binCtx, peer, uint8(bin), cur)
				}
				go p.liveSyncWorker(binCtx, peer, uint8(bin), cur)
			}
		}
		for _, bin := range dontWant {
			if c, ok := syncCtx.binCancelFuncs[bin]; ok {
				// we have sync running on this bin, cancel it
				c()
				delete(syncCtx.binCancelFuncs, bin)
			}
		}
	} else {
		// outside of depth
		var (
			want     = po
			dontWant = []uint8{0} // never want bin 0
		)

		for i := uint8(0); i < bins; i++ {
			if i == want {
				continue
			}
			dontWant = append(dontWant, i)
		}

		if _, ok := syncCtx.binCancelFuncs[want]; !ok {
			// if there's no bin cancel func it means there's no
			// sync running on this bin. start syncing both hist and live
			cur := c[want]
			binCtx, cancel := context.WithCancel(ctx)
			syncCtx.binCancelFuncs[po] = cancel
			if cur > 0 {
				go p.histSyncWorker(binCtx, peer, uint8(want), cur)
			}
			go p.liveSyncWorker(binCtx, peer, uint8(want), cur)
		}
		for _, bin := range dontWant {
			if c, ok := syncCtx.binCancelFuncs[bin]; ok {
				// we have sync running on this bin, cancel it
				c()
				delete(syncCtx.binCancelFuncs, bin)
			}
		}
	}
}

func (p *Puller) syncPeer(ctx context.Context, peer swarm.Address, po uint8, d uint8) {
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

	// peer outside depth?
	if po < d {
		cur, bin := c[po], po
		// start just one bin for historical and live
		binCtx, cancel := context.WithCancel(ctx)
		syncCtx.binCancelFuncs[po] = cancel
		go p.histSyncWorker(binCtx, peer, uint8(bin), cur)
		go p.liveSyncWorker(binCtx, peer, uint8(bin), cur)

		return
	}

	// start historical
	for bin, cur := range c {
		if bin == 0 || uint8(bin) < d {
			continue
		}
		binCtx, cancel := context.WithCancel(ctx)
		syncCtx.binCancelFuncs[uint8(bin)] = cancel
		if cur > 0 {
			go p.histSyncWorker(binCtx, peer, uint8(bin), cur)
		}
		go p.liveSyncWorker(binCtx, peer, uint8(bin), cur)
	}
}

func (p *Puller) histSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	p.logger.Debugf("histSyncWorker starting, peer %s bin %d cursor %d", peer, bin, cur)
	for {
		select {
		case <-p.quit:
			p.logger.Debugf("histSyncWorker quitting on shutdown. peer %s bin %d cur %d", peer, bin, cur)
			return
		case <-ctx.Done():
			p.logger.Debugf("histSyncWorker context cancelled. peer %s bin %d cur %d", peer, bin, cur)
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
			p.logger.Debugf("histSyncWorker finished syncing bin %d, cursor %d", bin, cur)
			return
		}
		start := time.Now()
		top, err := p.syncer.SyncInterval(ctx, peer, uint8(bin), s, cur)
		if err != nil {
			p.logger.Debugf("histSyncWorker error syncing interval. peer %s, bin %d, cursor %d, err %v", peer.String(), bin, cur, err)
			return
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
			p.logger.Debugf("liveSyncWorker quit on shutdown. peer %s bin %d cur %d", peer, bin, cur)
			return
		case <-ctx.Done():
			p.logger.Debugf("liveSyncWorker context cancelled. peer %s bin %d cur %d", peer, bin, cur)
			return
		default:
		}
		p.logger.Tracef("liveSyncWorker peer %s syncing bin %d from %d", peer, bin, from)
		top, err := p.syncer.SyncInterval(ctx, peer, bin, from, math.MaxUint64)
		if err != nil {
			p.logger.Debugf("liveSyncWorker exit on sync error. peer %s bin %d from %d err %v", peer, bin, from, err)
			return
		}
		if top == 0 {
			return
			panic(err)
		}
		p.logger.Tracef("liveSyncWorker peer %s synced bin %d from %d to %d", peer, bin, from, top)
		err = p.addPeerInterval(peer, bin, from, top)
		if err != nil {
			p.logger.Debugf("liveSyncWorker exit on add peer interval. peer %s bin %d from %d err %v", peer, bin, from, err)
			return
		}
		from = top + 1
	}
}

func (p *Puller) Close() error {
	close(p.quit)
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	for i := 0; i < bins; i++ {
		binPeers := p.syncPeers[i]
		for _, peer := range binPeers {
			peer.Lock()
			for _, f := range peer.binCancelFuncs {
				f()
			}
			peer.Unlock()
		}
	}

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
	binCancelFuncs map[uint8]func() // slice of context cancel funcs for historical sync. index is bin

	sync.Mutex
}

func newSyncPeer() *syncPeer {
	return &syncPeer{
		binCancelFuncs: make(map[uint8]func(), bins),
	}
}

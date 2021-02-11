// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package puller provides protocol-orchestrating functionality
// over the pullsync protocol. It pulls chunks from other nodes
// and reacts to changes in network configuration.
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

const defaultShallowBinPeers = 2

var (
	logMore = false // enable this to get more logging
)

type Options struct {
	Bins            uint8
	ShallowBinPeers int
}

type Puller struct {
	mtx         sync.Mutex
	topology    topology.Driver
	statestore  storage.StateStorer
	intervalMtx sync.Mutex
	syncer      pullsync.Interface

	metrics metrics
	logger  logging.Logger

	syncPeers    []map[string]*syncPeer // index is bin, map key is peer address
	syncPeersMtx sync.Mutex

	cursors    map[string][]uint64
	cursorsMtx sync.Mutex

	quit chan struct{}
	wg   sync.WaitGroup

	bins            uint8 // how many bins do we support
	shallowBinPeers int   // how many peers per bin do we want to sync with outside of depth
}

func New(stateStore storage.StateStorer, topology topology.Driver, pullSync pullsync.Interface, logger logging.Logger, o Options) *Puller {
	var (
		bins            uint8 = swarm.MaxBins
		shallowBinPeers int   = defaultShallowBinPeers
	)
	if o.Bins != 0 {
		bins = o.Bins
	}
	if o.ShallowBinPeers != 0 {
		shallowBinPeers = o.ShallowBinPeers
	}

	p := &Puller{
		statestore: stateStore,
		topology:   topology,
		syncer:     pullSync,
		metrics:    newMetrics(),
		logger:     logger,
		cursors:    make(map[string][]uint64),

		syncPeers: make([]map[string]*syncPeer, bins),
		quit:      make(chan struct{}),
		wg:        sync.WaitGroup{},

		bins:            bins,
		shallowBinPeers: shallowBinPeers,
	}

	for i := uint8(0); i < bins; i++ {
		p.syncPeers[i] = make(map[string]*syncPeer)
	}
	p.wg.Add(1)
	go p.manage()
	return p
}

type peer struct {
	addr swarm.Address
	po   uint8
}

func (p *Puller) manage() {
	defer p.wg.Done()
	c, unsubscribe := p.topology.SubscribePeersChange()
	defer unsubscribe()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-p.quit
		cancel()
	}()
	for {
		select {
		case <-c:
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
				peersToSync       []peer
				peersToRecalc     []peer
				peersDisconnected = make(map[string]peer)
			)

			p.syncPeersMtx.Lock()

			// make a map of all peers we're syncing with, then remove from it
			// the entries we get from kademlia  in the iterator, this way we
			// know which peers are no longer there anymore (disconnected) thus
			// should be removed from the syncPeer bin.
			for po, bin := range p.syncPeers {
				for peerAddr, v := range bin {
					pe := peer{addr: v.address, po: uint8(po)}
					peersDisconnected[peerAddr] = pe
				}
			}

			// EachPeerRev in this case will never return an error, since the content of the callback
			// never returns an error. In case in the future changes are made to the callback in a
			// way that it returns an error - the value must be checked.
			_ = p.topology.EachPeerRev(func(peerAddr swarm.Address, po uint8) (stop, jumpToNext bool, err error) {
				bp := p.syncPeers[po]
				if _, ok := bp[peerAddr.String()]; ok {
					delete(peersDisconnected, peerAddr.String())
				}
				syncing := len(bp)
				if po < depth {
					// outside of depth, sync peerPO bin only
					if _, ok := bp[peerAddr.String()]; !ok {
						if syncing < p.shallowBinPeers {
							// peer not syncing yet and we still need more peers in this bin
							bp[peerAddr.String()] = newSyncPeer(peerAddr, p.bins)
							peerEntry := peer{addr: peerAddr, po: po}
							peersToSync = append(peersToSync, peerEntry)
						}
					} else {
						// already syncing, recalc
						peerEntry := peer{addr: peerAddr, po: po}
						peersToRecalc = append(peersToRecalc, peerEntry)
					}
				} else {
					// within depth, sync everything >= depth
					if _, ok := bp[peerAddr.String()]; !ok {
						// we're not syncing with this peer yet, start doing so
						bp[peerAddr.String()] = newSyncPeer(peerAddr, p.bins)
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

			for _, v := range peersToSync {
				p.syncPeer(ctx, v.addr, v.po, depth)
			}

			for _, v := range peersToRecalc {
				p.recalcPeer(ctx, v.addr, v.po, depth)
			}

			for _, v := range peersDisconnected {
				p.disconnectPeer(ctx, v.addr, v.po)
			}

			p.syncPeersMtx.Unlock()

		case <-p.quit:
			return
		}
	}
}

func (p *Puller) disconnectPeer(ctx context.Context, peer swarm.Address, po uint8) {
	if logMore {
		p.logger.Debugf("puller disconnect cleanup peer %s po %d", peer, po)
	}
	syncCtx := p.syncPeers[po][peer.String()] // disconnectPeer is called under lock, this is safe
	syncCtx.gone()

	delete(p.syncPeers[po], peer.String())
}

func (p *Puller) recalcPeer(ctx context.Context, peer swarm.Address, po, d uint8) {
	if logMore {
		p.logger.Debugf("puller recalculating peer %s po %d depth %d", peer, po, d)
	}
	syncCtx := p.syncPeers[po][peer.String()] // recalcPeer is called under lock, this is safe

	syncCtx.Lock()
	defer syncCtx.Unlock()

	p.cursorsMtx.Lock()
	c := p.cursors[peer.String()]
	p.cursorsMtx.Unlock()

	if po >= d {
		// within depth
		var want, dontWant []uint8

		for i := d; i < p.bins; i++ {
			if i == 0 {
				continue
			}
			want = append(want, i)
		}
		for i := uint8(0); i < d; i++ {
			dontWant = append(dontWant, i)
		}

		for _, bin := range want {
			if !syncCtx.isBinSyncing(bin) {
				p.syncPeerBin(ctx, syncCtx, peer, bin, c[bin])
			}
		}
		syncCtx.cancelBins(dontWant...)
	} else {
		// outside of depth
		var (
			want     = po
			dontWant = []uint8{0} // never want bin 0
		)

		for i := uint8(0); i < p.bins; i++ {
			if i == want {
				continue
			}
			dontWant = append(dontWant, i)
		}

		if !syncCtx.isBinSyncing(want) {
			p.syncPeerBin(ctx, syncCtx, peer, want, c[want])
		}
		syncCtx.cancelBins(dontWant...)
	}
}

func (p *Puller) syncPeer(ctx context.Context, peer swarm.Address, po, d uint8) {
	syncCtx := p.syncPeers[po][peer.String()] // syncPeer is called under lock, so this is safe
	syncCtx.Lock()
	defer syncCtx.Unlock()

	p.cursorsMtx.Lock()
	c, ok := p.cursors[peer.String()]
	p.cursorsMtx.Unlock()

	if !ok {
		cursors, err := p.syncer.GetCursors(ctx, peer)
		if err != nil {
			if logMore {
				p.logger.Debugf("could not get cursors from peer %s: %v", peer.String(), err)
			}
			delete(p.syncPeers[po], peer.String())
			return
			// remove from syncing peers list, trigger channel to find some other peer
			// maybe blacklist for some time
		}
		p.cursorsMtx.Lock()
		p.cursors[peer.String()] = cursors
		p.cursorsMtx.Unlock()
		c = cursors
	}

	// peer outside depth?
	if po < d && po > 0 {
		p.syncPeerBin(ctx, syncCtx, peer, po, c[po])
		return
	}

	for bin, cur := range c {
		if bin == 0 || uint8(bin) < d {
			continue
		}
		p.syncPeerBin(ctx, syncCtx, peer, uint8(bin), cur)
	}
}

func (p *Puller) syncPeerBin(ctx context.Context, syncCtx *syncPeer, peer swarm.Address, bin uint8, cur uint64) {
	binCtx, cancel := context.WithCancel(ctx)
	syncCtx.setBinCancel(cancel, bin)
	if cur > 0 {
		p.wg.Add(1)
		go p.histSyncWorker(binCtx, peer, bin, cur)
	}
	// start live
	p.wg.Add(1)
	go p.liveSyncWorker(binCtx, peer, bin, cur)
}

func (p *Puller) histSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	defer func() {
		p.wg.Done()
		p.metrics.HistWorkerDoneCounter.Inc()
	}()
	if logMore {
		p.logger.Tracef("histSyncWorker starting, peer %s bin %d cursor %d", peer, bin, cur)
	}
	for {
		p.metrics.HistWorkerIterCounter.Inc()
		select {
		case <-p.quit:
			if logMore {
				p.logger.Tracef("histSyncWorker quitting on shutdown. peer %s bin %d cur %d", peer, bin, cur)
			}
			return
		case <-ctx.Done():
			if logMore {
				p.logger.Tracef("histSyncWorker context cancelled. peer %s bin %d cur %d", peer, bin, cur)
			}
			return
		default:
		}

		s, _, _, err := p.nextPeerInterval(peer, bin)
		if err != nil {
			p.metrics.HistWorkerErrCounter.Inc()
			p.logger.Debugf("histSyncWorker nextPeerInterval: %v", err)
			return
		}
		if s > cur {
			if logMore {
				p.logger.Tracef("histSyncWorker finished syncing bin %d, cursor %d", bin, cur)
			}
			return
		}
		top, ruid, err := p.syncer.SyncInterval(ctx, peer, bin, s, cur)
		if err != nil {
			if logMore {
				p.logger.Debugf("histSyncWorker error syncing interval. peer %s, bin %d, cursor %d, err %v", peer.String(), bin, cur, err)
			}
			if ruid == 0 {
				p.metrics.HistWorkerErrCounter.Inc()
				return
			}
			if err := p.syncer.CancelRuid(ctx, peer, ruid); err != nil && logMore {
				p.logger.Debugf("histSyncWorker cancel ruid: %v", err)
			}
			return
		}
		err = p.addPeerInterval(peer, bin, s, top)
		if err != nil {
			p.metrics.HistWorkerErrCounter.Inc()
			p.logger.Errorf("could not persist interval for peer %s, quitting", peer)
			return
		}
	}
}

func (p *Puller) liveSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	defer p.wg.Done()
	if logMore {
		p.logger.Tracef("liveSyncWorker starting, peer %s bin %d cursor %d", peer, bin, cur)
	}
	from := cur + 1
	for {
		p.metrics.LiveWorkerIterCounter.Inc()
		select {
		case <-p.quit:
			if logMore {
				p.logger.Tracef("liveSyncWorker quit on shutdown. peer %s bin %d cur %d", peer, bin, cur)
			}
			return
		case <-ctx.Done():
			if logMore {
				p.logger.Tracef("liveSyncWorker context cancelled. peer %s bin %d cur %d", peer, bin, cur)
			}
			return
		default:
		}
		top, ruid, err := p.syncer.SyncInterval(ctx, peer, bin, from, math.MaxUint64)
		if err != nil {
			if logMore {
				p.logger.Debugf("liveSyncWorker exit on sync error. peer %s bin %d from %d err %v", peer, bin, from, err)
			}
			if ruid == 0 {
				p.metrics.LiveWorkerErrCounter.Inc()
				return
			}
			if err := p.syncer.CancelRuid(ctx, peer, ruid); err != nil && logMore {
				p.logger.Debugf("histSyncWorker cancel ruid: %v", err)
			}
			return
		}
		err = p.addPeerInterval(peer, bin, from, top)
		if err != nil {
			p.metrics.LiveWorkerErrCounter.Inc()
			p.logger.Errorf("liveSyncWorker exit on add peer interval. peer %s bin %d from %d err %v", peer, bin, from, err)
			return
		}
		from = top + 1
	}
}

func (p *Puller) Close() error {
	p.logger.Info("puller shutting down")
	close(p.quit)
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	for i := uint8(0); i < p.bins; i++ {
		binPeers := p.syncPeers[i]
		for _, peer := range binPeers {
			peer.gone()
		}
	}
	cc := make(chan struct{})
	go func() {
		defer close(cc)
		p.wg.Wait()
	}()
	select {
	case <-cc:
	case <-time.After(10 * time.Second):
		p.logger.Warning("puller shutting down with running goroutines")
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
	address        swarm.Address
	binCancelFuncs map[uint8]func() // slice of context cancel funcs for historical sync. index is bin

	sync.Mutex
}

func newSyncPeer(addr swarm.Address, bins uint8) *syncPeer {
	return &syncPeer{
		address:        addr,
		binCancelFuncs: make(map[uint8]func(), bins),
	}
}

// called when peer disconnects or on shutdown, cleans up ongoing sync operations
func (p *syncPeer) gone() {
	p.Lock()
	defer p.Unlock()

	for _, f := range p.binCancelFuncs {
		f()
	}
}

func (p *syncPeer) setBinCancel(cf func(), bin uint8) {
	p.binCancelFuncs[bin] = cf
}

func (p *syncPeer) cancelBins(bins ...uint8) {
	for _, bin := range bins {
		if c, ok := p.binCancelFuncs[bin]; ok {
			c()
			delete(p.binCancelFuncs, bin)
		}
	}
}

func (p *syncPeer) isBinSyncing(bin uint8) bool {
	_, ok := p.binCancelFuncs[bin]
	return ok
}

func isSyncing(p *Puller, addr swarm.Address) bool {
	// this is needed for testing purposes in order
	// to verify that a peer is no longer syncing on
	// disconnect
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	for _, bin := range p.syncPeers {
		for peer := range bin {
			if addr.String() == peer {
				return true
			}
		}
	}
	return false
}

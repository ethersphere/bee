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
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/pullsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "puller"

const (
	cursorPruneTimeout = 24 * time.Hour
)

type Options struct {
	Bins uint8
}

type Puller struct {
	topology   topology.Driver
	statestore storage.StateStorer
	syncer     pullsync.Interface

	metrics metrics
	logger  log.Logger

	syncPeers    []map[string]*syncPeer // index is bin, map key is peer address
	syncPeersMtx sync.Mutex

	cursors    map[string]peerCursors
	cursorsMtx sync.Mutex

	quit chan struct{}
	wg   sync.WaitGroup

	bins uint8 // how many bins do we support
}

func New(stateStore storage.StateStorer, topology topology.Driver, pullSync pullsync.Interface, logger log.Logger, o Options, warmupTime time.Duration) *Puller {
	var (
		bins uint8 = swarm.MaxBins
	)
	if o.Bins != 0 {
		bins = o.Bins
	}

	p := &Puller{
		statestore: stateStore,
		topology:   topology,
		syncer:     pullSync,
		metrics:    newMetrics(),
		logger:     logger.WithName(loggerName).Register(),
		cursors:    make(map[string]peerCursors),

		syncPeers: make([]map[string]*syncPeer, bins),
		quit:      make(chan struct{}),
		wg:        sync.WaitGroup{},

		bins: bins,
	}

	for i := uint8(0); i < bins; i++ {
		p.syncPeers[i] = make(map[string]*syncPeer)
	}
	p.wg.Add(1)
	go p.manage(warmupTime)
	return p
}

type peer struct {
	addr swarm.Address
	po   uint8
}

func (p *Puller) manage(warmupTime time.Duration) {
	defer p.wg.Done()
	c, unsubscribe := p.topology.SubscribePeersChange()
	defer unsubscribe()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-p.quit
		cancel()
	}()

	// wait for warmup duration to complete
	select {
	case <-time.After(warmupTime):
	case <-p.quit:
		return
	}

	p.logger.Info("puller: warmup period complete, worker starting.")

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
				if po >= depth {
					// delete from peersDisconnected since we'd like to sync
					// with this peer
					delete(peersDisconnected, peerAddr.ByteString())

					// within depth, sync everything
					if _, ok := bp[peerAddr.ByteString()]; !ok {
						// we're not syncing with this peer yet, start doing so
						bp[peerAddr.ByteString()] = newSyncPeer(peerAddr, p.bins)
						peerEntry := peer{addr: peerAddr, po: po}
						peersToSync = append(peersToSync, peerEntry)
					} else {
						// already syncing, recalc
						peerEntry := peer{addr: peerAddr, po: po}
						peersToRecalc = append(peersToRecalc, peerEntry)
					}
				}

				// if peer is outside of depth, do nothing here, this
				// will cause the peer to stay in the peersDisconnected
				// map, leading to cancelling of its running syncing contexts.

				return false, false, nil
			}, topology.Filter{Reachable: true})

			p.syncPeersMtx.Unlock()

			for _, v := range peersToSync {
				p.syncPeer(ctx, v.addr, v.po, depth)
			}

			for _, v := range peersToRecalc {
				dontSync := p.recalcPeer(ctx, v.addr, v.po, depth)
				// stopgap solution for peers that dont return the correct
				// amount of cursors we expect
				if dontSync {
					peersDisconnected[v.addr.ByteString()] = v
				}
			}

			p.syncPeersMtx.Lock()
			for _, v := range peersDisconnected {
				p.disconnectPeer(v.addr, v.po)
			}
			p.syncPeersMtx.Unlock()

		case <-p.quit:
			return
		}
	}
}

func (p *Puller) disconnectPeer(peer swarm.Address, po uint8) {
	loggerV2 := p.logger.V(2).Register()

	loggerV2.Debug("puller disconnect cleanup peer", "peer_address", peer, "proximity_order", po)
	if syncCtx, ok := p.syncPeers[po][peer.ByteString()]; ok {
		// disconnectPeer is called under lock, this is safe
		syncCtx.gone()
	}
	delete(p.syncPeers[po], peer.ByteString())

	// delete the peer cursors
	p.cursorsMtx.Lock()
	if c, ok := p.cursors[peer.ByteString()]; ok && c.created.Add(cursorPruneTimeout).After(time.Now()) {
		delete(p.cursors, peer.ByteString())
	}
	p.cursorsMtx.Unlock()
}

func (p *Puller) recalcPeer(ctx context.Context, peer swarm.Address, po, d uint8) (dontSync bool) {
	loggerV2 := p.logger.V(2).Register()

	loggerV2.Debug("puller recalculating peer", "peer_address", peer, "proximity_order", po, "depth", d)

	p.syncPeersMtx.Lock()
	syncCtx := p.syncPeers[po][peer.ByteString()]
	p.syncPeersMtx.Unlock()

	syncCtx.Lock()
	defer syncCtx.Unlock()

	p.cursorsMtx.Lock()
	c := p.cursors[peer.ByteString()].cursors
	p.cursorsMtx.Unlock()

	if len(c) != int(p.bins) {
		return true
	}

	var want, dontWant []uint8
	if po >= d {
		// within depth
		for i := d; i < p.bins; i++ {
			if i == 0 {
				continue
			}
			want = append(want, i)
		}

		for _, bin := range want {
			if !syncCtx.isBinSyncing(bin) {
				p.syncPeerBin(ctx, syncCtx, peer, bin, c[bin])
			}
		}

		// cancel everything outside of depth
		for i := uint8(0); i < d; i++ {
			dontWant = append(dontWant, i)
		}
	} else {
		// peer is outside depth. cancel everything
		for i := uint8(0); i < p.bins; i++ {
			dontWant = append(dontWant, i)
		}
	}

	syncCtx.cancelBins(dontWant...)
	return false
}

func (p *Puller) syncPeer(ctx context.Context, peer swarm.Address, po, d uint8) {
	loggerV2 := p.logger.V(2).Register()

	p.syncPeersMtx.Lock()
	syncCtx := p.syncPeers[po][peer.ByteString()]
	p.syncPeersMtx.Unlock()

	syncCtx.Lock()
	defer syncCtx.Unlock()

	p.cursorsMtx.Lock()
	c, ok := p.cursors[peer.ByteString()]
	p.cursorsMtx.Unlock()

	if !ok {
		cursors, err := p.syncer.GetCursors(ctx, peer)
		if err != nil {
			loggerV2.Debug("could not get cursors from peer", "peer_address", peer, "error", err)
			p.syncPeersMtx.Lock()
			delete(p.syncPeers[po], peer.ByteString())
			p.syncPeersMtx.Unlock()

			return
			// remove from syncing peers list, trigger channel to find some other peer
			// maybe blacklist for some time
		}
		p.cursorsMtx.Lock()
		p.cursors[peer.ByteString()] = peerCursors{created: time.Now(), cursors: cursors}
		c = p.cursors[peer.ByteString()]
		p.cursorsMtx.Unlock()
	}

	// if length of returned cursors does not add up to
	// what we expect it to be - dont do anything
	if len(c.cursors) != int(p.bins) {
		p.syncPeersMtx.Lock()
		delete(p.syncPeers[po], peer.ByteString())
		p.syncPeersMtx.Unlock()
		return
	}

	for bin, cur := range c.cursors {
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
	loggerV2 := p.logger.V(2).Register()

	defer func() {
		p.wg.Done()
		p.metrics.HistWorkerDoneCounter.Inc()
	}()
	loggerV2.Debug("histSyncWorker starting", "peer_address", peer, "bin", bin, "cursor", cur)
	for {
		p.metrics.HistWorkerIterCounter.Inc()
		select {
		case <-p.quit:
			loggerV2.Debug("histSyncWorker quitting on shutdown", "peer_address", peer, "bin", bin, "cursor", cur)
			return
		case <-ctx.Done():
			loggerV2.Debug("histSyncWorker context cancelled", "peer_address", peer, "bin", bin, "cursor", cur)
			return
		default:
		}

		s, _, _, err := p.nextPeerInterval(peer, bin)
		if err != nil {
			p.metrics.HistWorkerErrCounter.Inc()
			p.logger.Debug("histSyncWorker nextPeerInterval failed", "error", err)
			return
		}
		if s > cur {
			loggerV2.Debug("histSyncWorker syncing finished", "bin", bin, "cursor", cur)
			return
		}
		top, ruid, err := p.syncer.SyncInterval(ctx, peer, bin, s, cur)
		if err != nil {
			loggerV2.Debug("histSyncWorker syncing interval failed", "peer_address", peer, "bin", bin, "cursor", cur, "error", err)
			if ruid == 0 {
				p.metrics.HistWorkerErrCounter.Inc()
			}

			// since we use bin context cancellation to cancel interval
			// sync operations, the context can be expired here, causing us
			// to try to send a message with an expired context, which is
			// bound to fail.
			ctxC, cancelC := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelC()
			if err := p.syncer.CancelRuid(ctxC, peer, ruid); err != nil {
				loggerV2.Debug("histSyncWorker cancel ruid failed", "error", err)
			}
			return
		}
		err = p.addPeerInterval(peer, bin, s, top)
		if err != nil {
			p.metrics.HistWorkerErrCounter.Inc()
			p.logger.Error(err, "could not persist interval for peer, quitting...", "peer_address", peer)
			return
		}
		loggerV2.Debug("histSyncWorker pulled bin %d [%d:%d], peer %s", "bin", bin, "start", s, "topmost", top, "peer_address", peer)
	}
}

func (p *Puller) liveSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	loggerV2 := p.logger.V(2).Register()

	defer p.wg.Done()
	loggerV2.Debug("liveSyncWorker starting", "peer_address", peer, "bin", bin, "cursor", cur)
	from := cur + 1
	for {
		p.metrics.LiveWorkerIterCounter.Inc()
		select {
		case <-p.quit:
			loggerV2.Debug("liveSyncWorker quit on shutdown", "peer_address", peer, "bin", bin, "cursor", cur)
			return
		case <-ctx.Done():
			loggerV2.Debug("liveSyncWorker context cancelled", "peer_address", peer, "bin", bin, "cursor", cur)
			return
		default:
		}
		top, ruid, err := p.syncer.SyncInterval(ctx, peer, bin, from, math.MaxUint64)
		if err != nil {
			loggerV2.Debug("liveSyncWorker exit on sync error", "peer_address", peer, "bin", bin, "from", from, "error", err)
			if ruid == 0 {
				p.metrics.LiveWorkerErrCounter.Inc()
			}

			// since we use bin context cancellation to cancel interval
			// sync operations, the context can be expired here, causing us
			// to try to send a message with an expired context, which is
			// bound to fail.
			ctxC, cancelC := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelC()
			if err := p.syncer.CancelRuid(ctxC, peer, ruid); err != nil {
				loggerV2.Debug("histSyncWorker cancel ruid failed", "error", err)
			}
			return
		}
		if top == math.MaxUint64 {
			p.metrics.MaxUintErrCounter.Inc()
			return
		}
		err = p.addPeerInterval(peer, bin, from, top)
		if err != nil {
			p.metrics.LiveWorkerErrCounter.Inc()
			p.logger.Error(err, "liveSyncWorker exit on add peer interval", "peer_address", peer, "bin", bin, "from", from, "error", err)
			return
		}
		loggerV2.Debug("liveSyncWorker pulled bin failed", "bin", bin, "from", from, "topmost", top, "peer_address", peer)

		from = top + 1
	}
}

func (p *Puller) Close() error {
	p.logger.Info("puller shutting down")
	close(p.quit)
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

	peerStreamKey := peerIntervalKey(peer, bin)
	i, err := p.getOrCreateInterval(peer, bin)
	if err != nil {
		return err
	}

	i.Add(start, end)

	return p.statestore.Put(peerStreamKey, i)
}

func (p *Puller) nextPeerInterval(peer swarm.Address, bin uint8) (start, end uint64, empty bool, err error) {

	i, err := p.getOrCreateInterval(peer, bin)
	if err != nil {
		return 0, 0, false, err
	}

	start, end, empty = i.Next(0)
	return start, end, empty, nil
}

func (p *Puller) getOrCreateInterval(peer swarm.Address, bin uint8) (*intervalstore.Intervals, error) {
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
			if addr.ByteString() == peer {
				return true
			}
		}
	}
	return false
}

type peerCursors struct {
	created time.Time
	cursors []uint64
}

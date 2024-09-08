// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package puller provides protocol-orchestrating functionality
// over the pullsync protocol. It pulls chunks from other nodes
// and reacts to changes in network configuration.
package puller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/puller/intervalstore"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/rate"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	ratelimit "golang.org/x/time/rate"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "puller"

var errCursorsLength = errors.New("cursors length mismatch")

const (
	DefaultHistRateWindow = time.Minute * 15

	IntervalPrefix = "sync_interval"
	recalcPeersDur = time.Minute * 5

	maxChunksPerSecond = 1000 // roughly 4 MB/s

	maxPODelta = 2 // the lowest level of proximity order (of peers) subtracted from the storage radius allowed for chunk syncing.
)

type Options struct {
	Bins uint8
}

type Puller struct {
	base swarm.Address

	topology    topology.Driver
	radius      storer.RadiusChecker
	statestore  storage.StateStorer
	syncer      pullsync.Interface
	blockLister p2p.Blocklister

	metrics metrics
	logger  log.Logger

	syncPeers    map[string]*syncPeer // index is bin, map key is peer address
	syncPeersMtx sync.Mutex
	intervalMtx  sync.Mutex

	cancel func()

	wg sync.WaitGroup

	bins uint8 // how many bins do we support

	rate *rate.Rate // rate of historical syncing

	start sync.Once

	limiter *ratelimit.Limiter
}

func New(
	addr swarm.Address,
	stateStore storage.StateStorer,
	topology topology.Driver,
	reserveState storer.RadiusChecker,
	pullSync pullsync.Interface,
	blockLister p2p.Blocklister,
	logger log.Logger,
	o Options,
) *Puller {
	bins := swarm.MaxBins
	if o.Bins != 0 {
		bins = o.Bins
	}
	p := &Puller{
		base:        addr,
		statestore:  stateStore,
		topology:    topology,
		radius:      reserveState,
		syncer:      pullSync,
		metrics:     newMetrics(),
		logger:      logger.WithName(loggerName).Register(),
		syncPeers:   make(map[string]*syncPeer),
		bins:        bins,
		blockLister: blockLister,
		rate:        rate.New(DefaultHistRateWindow),
		cancel:      func() { /* Noop, since the context is initialized in the Start(). */ },
		limiter:     ratelimit.NewLimiter(ratelimit.Every(time.Second/maxChunksPerSecond), maxChunksPerSecond),
	}

	return p
}

func (p *Puller) Start(ctx context.Context) {
	p.start.Do(func() {
		cctx, cancel := context.WithCancel(ctx)
		p.cancel = cancel

		p.wg.Add(1)
		go p.manage(cctx)
	})
}

func (p *Puller) SyncRate() float64 {
	return p.rate.Rate()
}

func (p *Puller) manage(ctx context.Context) {
	defer p.wg.Done()

	c, unsubscribe := p.topology.SubscribeTopologyChange()
	defer unsubscribe()

	p.logger.Info("warmup period complete, starting worker")

	var prevRadius uint8

	onChange := func() {
		p.syncPeersMtx.Lock()
		defer p.syncPeersMtx.Unlock()

		newRadius := p.radius.StorageRadius()

		// reset all intervals below the new radius to resync:
		// 1. previously evicted chunks
		// 2. previously ignored chunks due to a higher radius
		if newRadius < prevRadius {
			for _, peer := range p.syncPeers {
				p.disconnectPeer(peer.address)
			}
			if err := p.resetIntervals(prevRadius); err != nil {
				p.logger.Debug("reset lower sync radius failed", "error", err)
			}
			p.logger.Debug("radius decrease", "old_radius", prevRadius, "new_radius", newRadius)
		}
		prevRadius = newRadius

		// peersDisconnected is used to mark and prune peers that are no longer connected.
		peersDisconnected := maps.Clone(p.syncPeers)

		_ = p.topology.EachConnectedPeerRev(func(addr swarm.Address, po uint8) (stop, jumpToNext bool, err error) {
			if _, ok := p.syncPeers[addr.ByteString()]; !ok {
				p.syncPeers[addr.ByteString()] = newSyncPeer(addr, p.bins, po)
			}
			delete(peersDisconnected, addr.ByteString())
			return false, false, nil
		}, topology.Select{})

		for _, peer := range peersDisconnected {
			p.disconnectPeer(peer.address)
		}

		p.recalcPeers(ctx, newRadius)
	}

	tick := time.NewTicker(recalcPeersDur)
	defer tick.Stop()

	for {

		onChange()

		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-c:
		}
	}
}

// disconnectPeer cancels all existing syncing and removes the peer entry from the syncing map.
// Must be called under lock.
func (p *Puller) disconnectPeer(addr swarm.Address) {
	loggerV2 := p.logger.V(2).Register()

	loggerV2.Debug("disconnecting peer", "peer_address", addr)
	if peer, ok := p.syncPeers[addr.ByteString()]; ok {
		peer.mtx.Lock()
		peer.stop()
		peer.mtx.Unlock()
	}
	delete(p.syncPeers, addr.ByteString())
}

// recalcPeers starts or stops syncing process for peers per bin depending on the current sync radius.
// Must be called under lock.
func (p *Puller) recalcPeers(ctx context.Context, storageRadius uint8) {
	var wg sync.WaitGroup
	for _, peer := range p.syncPeers {
		wg.Add(1)
		p.wg.Add(1)
		go func(peer *syncPeer) {
			defer p.wg.Done()
			defer wg.Done()
			if err := p.syncPeer(ctx, peer, storageRadius); err != nil {
				p.logger.Debug("sync peer failed", "peer_address", peer.address, "error", err)
			}
		}(peer)
	}
	wg.Wait()
}

func (p *Puller) syncPeer(ctx context.Context, peer *syncPeer, storageRadius uint8) error {
	peer.mtx.Lock()
	defer peer.mtx.Unlock()

	if peer.cursors == nil {
		cursors, epoch, err := p.syncer.GetCursors(ctx, peer.address)
		if err != nil {
			return fmt.Errorf("could not get cursors from peer %s: %w", peer.address, err)
		}
		peer.cursors = cursors

		storedEpoch, err := p.getPeerEpoch(peer.address)
		if err != nil {
			return fmt.Errorf("retrieve epoch for peer %s: %w", peer.address, err)
		}

		if storedEpoch != epoch {
			// cancel all bins
			peer.stop()

			p.logger.Debug("peer epoch change detected, resetting past synced intervals", "stored_epoch", storedEpoch, "new_epoch", epoch, "peer_address", peer.address)

			err = p.resetPeerIntervals(peer.address)
			if err != nil {
				return fmt.Errorf("reset intervals for peer %s: %w", peer.address, err)
			}
			err = p.setPeerEpoch(peer.address, epoch)
			if err != nil {
				return fmt.Errorf("set epoch for peer %s: %w", peer.address, err)
			}
		}
	}

	if len(peer.cursors) != int(p.bins) {
		return errCursorsLength
	}

	/*
		The syncing behavior diverges for peers outside and within the storage radius.
		For neighbor peers, we sync ALL bins greater than or equal to the storage radius.
		For peers with PO lower than the storage radius, we must sync ONLY the bin that is the PO.
		For peers peer with PO lower than the storage radius and even lower than the allowed minimum threshold,
		no syncing is done.
	*/

	if peer.po >= storageRadius {

		// cancel all bins lower than the storage radius
		for bin := uint8(0); bin < storageRadius; bin++ {
			peer.cancelBin(bin)
		}

		// sync all bins >= storage radius
		for bin, cur := range peer.cursors {
			if bin >= int(storageRadius) && !peer.isBinSyncing(uint8(bin)) {
				p.syncPeerBin(ctx, peer, uint8(bin), cur)
			}
		}

	} else if storageRadius-peer.po <= maxPODelta {
		// cancel all non-po bins, if any
		for bin := uint8(0); bin < p.bins; bin++ {
			if bin != peer.po {
				peer.cancelBin(bin)
			}
		}
		// sync PO bin only
		if !peer.isBinSyncing(peer.po) {
			p.syncPeerBin(ctx, peer, peer.po, peer.cursors[peer.po])
		}
	} else {
		peer.stop()
	}

	return nil
}

// syncPeerBin will start historical and live syncing for the peer for a particular bin.
// Must be called under syncPeer lock.
func (p *Puller) syncPeerBin(parentCtx context.Context, peer *syncPeer, bin uint8, cursor uint64) {
	loggerV2 := p.logger.V(2).Register()

	ctx, cancel := context.WithCancel(parentCtx)
	peer.setBinCancel(cancel, bin)

	sync := func(isHistorical bool, address swarm.Address, start uint64) {
		p.metrics.SyncWorkerCounter.Inc()

		defer p.wg.Done()
		defer peer.wg.Done()
		defer p.metrics.SyncWorkerCounter.Dec()

		var err error

		for {
			if isHistorical { // override start with the next interval if historical syncing
				start, err = p.nextPeerInterval(address, bin)
				if err != nil {
					p.metrics.SyncWorkerErrCounter.Inc()
					p.logger.Error(err, "syncWorker nextPeerInterval failed, quitting")
					return
				}

				// historical sync has caught up to the cursor, exit
				if start > cursor {
					return
				}
			}

			select {
			case <-ctx.Done():
				loggerV2.Debug("syncWorker context cancelled", "peer_address", address, "bin", bin)
				return
			default:
			}

			p.metrics.SyncWorkerIterCounter.Inc()

			syncStart := time.Now()
			top, count, err := p.syncer.Sync(ctx, address, bin, start)

			if top == math.MaxUint64 {
				p.metrics.MaxUintErrCounter.Inc()
				p.logger.Error(nil, "syncWorker max uint64 encountered, quitting", "peer_address", address, "bin", bin, "from", start, "topmost", top)
				return
			}

			if err != nil {
				p.metrics.SyncWorkerErrCounter.Inc()
				if errors.Is(err, p2p.ErrPeerNotFound) {
					p.logger.Debug("syncWorker interval failed, quitting", "error", err, "peer_address", address, "bin", bin, "cursor", cursor, "start", start, "topmost", top)
					return
				}
				loggerV2.Debug("syncWorker interval failed", "error", err, "peer_address", address, "bin", bin, "cursor", cursor, "start", start, "topmost", top)
			}

			_ = p.limiter.WaitN(ctx, count)

			if isHistorical {
				p.metrics.SyncedCounter.WithLabelValues("historical").Add(float64(count))
				p.rate.Add(count)
			} else {
				p.metrics.SyncedCounter.WithLabelValues("live").Add(float64(count))
			}

			// pulled at least one chunk
			if top >= start {
				if err := p.addPeerInterval(address, bin, start, top); err != nil {
					p.metrics.SyncWorkerErrCounter.Inc()
					p.logger.Error(err, "syncWorker could not persist interval for peer, quitting", "peer_address", address)
					return
				}
				loggerV2.Debug("syncWorker pulled", "bin", bin, "start", start, "topmost", top, "isHistorical", isHistorical, "duration", time.Since(syncStart), "peer_address", address)
				start = top + 1
			}
		}
	}

	if cursor > 0 {
		peer.wg.Add(1)
		p.wg.Add(1)
		go sync(true, peer.address, cursor)
	}

	peer.wg.Add(1)
	p.wg.Add(1)
	go sync(false, peer.address, cursor+1)
}

func (p *Puller) Close() error {
	p.logger.Info("shutting down")
	p.cancel()
	cc := make(chan struct{})
	go func() {
		defer close(cc)
		p.wg.Wait()
	}()
	select {
	case <-cc:
	case <-time.After(10 * time.Second):
		p.logger.Warning("shut down timeout, some goroutines may still be running")
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

func (p *Puller) getPeerEpoch(peer swarm.Address) (uint64, error) {
	p.intervalMtx.Lock()
	defer p.intervalMtx.Unlock()

	var epoch uint64
	err := p.statestore.Get(peerEpochKey(peer), &epoch)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}

	return epoch, nil
}

func (p *Puller) setPeerEpoch(peer swarm.Address, epoch uint64) error {
	p.intervalMtx.Lock()
	defer p.intervalMtx.Unlock()

	return p.statestore.Put(peerEpochKey(peer), epoch)
}

func (p *Puller) resetPeerIntervals(peer swarm.Address) (err error) {
	p.intervalMtx.Lock()
	defer p.intervalMtx.Unlock()

	for bin := uint8(0); bin < p.bins; bin++ {
		err = errors.Join(err, p.statestore.Delete(peerIntervalKey(peer, bin)))
	}

	return
}

func (p *Puller) resetIntervals(oldRadius uint8) (err error) {
	p.intervalMtx.Lock()
	defer p.intervalMtx.Unlock()

	var deleteKeys []string

	for bin := uint8(0); bin < p.bins; bin++ {
		err = errors.Join(err,
			p.statestore.Iterate(binIntervalKey(bin), func(key, _ []byte) (stop bool, err error) {

				po := swarm.Proximity(addressFromKey(key).Bytes(), p.base.Bytes())

				// 1. for neighbor peers, only reset the bins below the current radius
				// 2. for non-neighbor peers, we must reset the entire history
				if po >= oldRadius {
					if bin < oldRadius {
						deleteKeys = append(deleteKeys, string(key))
					}
				} else {
					deleteKeys = append(deleteKeys, string(key))
				}
				return false, nil
			}),
		)
	}

	for _, k := range deleteKeys {
		err = errors.Join(err, p.statestore.Delete(k))
	}

	return err
}

func (p *Puller) nextPeerInterval(peer swarm.Address, bin uint8) (uint64, error) {
	p.intervalMtx.Lock()
	defer p.intervalMtx.Unlock()

	i, err := p.getOrCreateInterval(peer, bin)
	if err != nil {
		return 0, err
	}

	start, _, _ := i.Next(0)
	return start, nil
}

// Must be called underlock.
func (p *Puller) getOrCreateInterval(peer swarm.Address, bin uint8) (*intervalstore.Intervals, error) {
	// check that an interval entry exists
	key := peerIntervalKey(peer, bin)
	itv := &intervalstore.Intervals{}
	if err := p.statestore.Get(key, itv); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// key interval values are ALWAYS > 0
			itv = intervalstore.NewIntervals(1)
			if err := p.statestore.Put(key, itv); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return itv, nil
}

func peerEpochKey(peer swarm.Address) string {
	return fmt.Sprintf("%s_epoch_%s", IntervalPrefix, peer.ByteString())
}

func peerIntervalKey(peer swarm.Address, bin uint8) string {
	return fmt.Sprintf("%s_%03d_%s", IntervalPrefix, bin, peer.ByteString())
}

func binIntervalKey(bin uint8) string {
	return fmt.Sprintf("%s_%03d", IntervalPrefix, bin)
}

func addressFromKey(key []byte) swarm.Address {
	addr := key[len(fmt.Sprintf("%s_%03d_", IntervalPrefix, 0)):]
	return swarm.NewAddress(addr)
}

type syncPeer struct {
	address        swarm.Address
	binCancelFuncs map[uint8]func() // slice of context cancel funcs for historical sync. index is bin
	po             uint8
	cursors        []uint64

	mtx sync.Mutex
	wg  sync.WaitGroup
}

func newSyncPeer(addr swarm.Address, bins, po uint8) *syncPeer {
	return &syncPeer{
		address:        addr,
		binCancelFuncs: make(map[uint8]func(), bins),
		po:             po,
	}
}

// called when peer disconnects or on shutdown, cleans up ongoing sync operations
func (p *syncPeer) stop() {
	for bin, c := range p.binCancelFuncs {
		c()
		delete(p.binCancelFuncs, bin)
	}
	p.wg.Wait()
}

func (p *syncPeer) setBinCancel(cf func(), bin uint8) {
	p.binCancelFuncs[bin] = cf
}

func (p *syncPeer) cancelBin(bin uint8) {
	if c, ok := p.binCancelFuncs[bin]; ok {
		c()
		delete(p.binCancelFuncs, bin)
	}
}

func (p *syncPeer) isBinSyncing(bin uint8) bool {
	_, ok := p.binCancelFuncs[bin]
	return ok
}

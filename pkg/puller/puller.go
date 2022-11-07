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
	"math"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/intervalstore"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pullsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "puller"

var errCursorsLength = errors.New("cursors length mismatch")

const DefaultSyncSleepDur = time.Minute

type Options struct {
	Bins         uint8
	SyncSleepDur time.Duration
}

type Puller struct {
	topology     topology.Driver
	reserveState postage.ReserveStateGetter
	statestore   storage.StateStorer
	syncer       pullsync.Interface

	metrics metrics
	logger  log.Logger

	syncPeers    []map[string]*syncPeer // index is bin, map key is peer address
	syncPeersMtx sync.Mutex

	cancel func()

	wg sync.WaitGroup

	syncSleepDur time.Duration

	bins uint8 // how many bins do we support
}

func New(stateStore storage.StateStorer, topology topology.Driver, reserveState postage.ReserveStateGetter, pullSync pullsync.Interface, logger log.Logger, o Options, warmupTime time.Duration) *Puller {
	var (
		bins uint8 = swarm.MaxBins
	)
	if o.Bins != 0 {
		bins = o.Bins
	}

	p := &Puller{
		statestore:   stateStore,
		topology:     topology,
		reserveState: reserveState,
		syncer:       pullSync,
		metrics:      newMetrics(),
		logger:       logger.WithName(loggerName).Register(),
		syncPeers:    make([]map[string]*syncPeer, bins),
		syncSleepDur: o.SyncSleepDur,
		bins:         bins,
	}

	for i := uint8(0); i < bins; i++ {
		p.syncPeers[i] = make(map[string]*syncPeer)
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	p.wg.Add(1)
	go p.manage(ctx, warmupTime)
	return p
}

func (p *Puller) manage(ctx context.Context, warmupTime time.Duration) {
	defer p.wg.Done()

	select {
	case <-time.After(warmupTime):
	case <-ctx.Done():
		return
	}

	c, unsubscribe := p.topology.SubscribeTopologyChange()
	defer unsubscribe()

	p.logger.Info("puller: warmup period complete, worker starting.")

	for {
		select {
		case <-ctx.Done():
			return
		case <-c:

			p.syncPeersMtx.Lock()

			// peersDisconnected is used to mark and prune peers that are no longer connected.
			peersDisconnected := make(map[string]*syncPeer)
			for _, bin := range p.syncPeers {
				for addr, peer := range bin {
					peersDisconnected[addr] = peer
				}
			}

			neighborhoodDepth := p.topology.NeighborhoodDepth()
			syncRadius := p.reserveState.GetReserveState().StorageRadius

			_ = p.topology.EachPeerRev(func(addr swarm.Address, po uint8) (stop, jumpToNext bool, err error) {
				if po >= neighborhoodDepth {
					// add peer to sync
					if _, ok := p.syncPeers[po][addr.ByteString()]; !ok {
						p.syncPeers[po][addr.ByteString()] = newSyncPeer(addr, po, p.bins)
					}
					// remove from disconnected list as the peer is still connected
					delete(peersDisconnected, addr.ByteString())
				}
				return false, false, nil
			}, topology.Filter{Reachable: true})

			for _, peer := range peersDisconnected {
				p.disconnectPeer(peer.address, peer.po)
			}

			p.recalcPeers(ctx, syncRadius)

			p.syncPeersMtx.Unlock()
		}
	}
}

// disconnectPeer cancels all existing syncing and removes the peer entry from the syncing map.
// Must be called under lock.
func (p *Puller) disconnectPeer(addr swarm.Address, po uint8) {
	loggerV2 := p.logger

	loggerV2.Debug("puller disconnect cleanup peer", "peer_address", addr, "proximity_order", po)
	if peer, ok := p.syncPeers[po][addr.ByteString()]; ok {
		peer.gone()

	}
	delete(p.syncPeers[po], addr.ByteString())
}

// recalcPeers starts or stops syncing process for peers per bin depending on the current sync radius.
// Must be called under lock.
func (p *Puller) recalcPeers(ctx context.Context, syncRadius uint8) {
	loggerV2 := p.logger

	for _, peers := range p.syncPeers {
		for _, peer := range peers {
			peer.Lock()
			for bin := uint8(0); bin < syncRadius; bin++ {
				peer.cancelBin(bin)
			}
			err := p.syncPeer(ctx, peer, syncRadius)
			if err != nil {
				loggerV2.Error(err, "recalc peers sync failed", "bin", syncRadius, "peer", peer.address)
			}
			peer.Unlock()
		}
	}
}

// Must be called under lock.
func (p *Puller) syncPeer(ctx context.Context, peer *syncPeer, syncRadius uint8) error {
	if peer.cursors == nil {
		cursors, err := p.syncer.GetCursors(ctx, peer.address)
		if err != nil {
			return fmt.Errorf("could not get cursors from peer %s: %w", peer.address, err)
		}
		peer.cursors = cursors
	}

	if len(peer.cursors) != int(p.bins) {
		return errCursorsLength
	}

	for bin, cur := range peer.cursors {
		if bin >= int(syncRadius) && !peer.isBinSyncing(uint8(bin)) {
			p.syncPeerBin(ctx, peer, uint8(bin), cur)
		}
	}

	return nil
}

// syncPeerBin will start historical and live syncing for the peer for a particular bin.
// Must be called under syncPeer lock.
func (p *Puller) syncPeerBin(ctx context.Context, peer *syncPeer, bin uint8, cur uint64) {
	binCtx, cancel := context.WithCancel(ctx)
	peer.setBinCancel(cancel, bin)
	if cur > 0 {
		p.wg.Add(1)
		go p.histSyncWorker(binCtx, peer.address, bin, cur)
	}
	// start live
	p.wg.Add(1)
	go p.liveSyncWorker(binCtx, peer.address, bin, cur)
}

func (p *Puller) histSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	loggerV2 := p.logger

	defer p.wg.Done()
	defer p.metrics.HistWorkerDoneCounter.Inc()

	sleep := false

	p.logger.Debug("histSyncWorker starting", "peer_address", peer, "bin", bin, "cursor", cur)

	for {
		p.metrics.HistWorkerIterCounter.Inc()

		if sleep {
			select {
			case <-ctx.Done():
				loggerV2.Debug("histSyncWorker context cancelled", "peer_address", peer, "bin", bin, "cursor", cur)
				return
			case <-time.After(p.syncSleepDur):
			}
			sleep = false
		}

		select {
		case <-ctx.Done():
			loggerV2.Debug("histSyncWorker context cancelled", "peer_address", peer, "bin", bin, "cursor", cur)
			return
		default:
		}

		s, _, _, err := p.nextPeerInterval(peer, bin)
		if err != nil {
			p.metrics.HistWorkerErrCounter.Inc()
			p.logger.Error(err, "histSyncWorker nextPeerInterval failed")
			return
		}

		if s > cur {
			p.logger.Debug("histSyncWorker syncing finished", "bin", bin, "cursor", cur)
			return
		}
		top, err := p.syncer.SyncInterval(ctx, peer, bin, s, cur)
		if err != nil {
			p.metrics.HistWorkerErrCounter.Inc()
			p.logger.Error(err, "histSyncWorker syncing interval failed", "peer_address", peer, "bin", bin, "cursor", cur, "start", s, "topmost", top)
			sleep = true
		}

		err = p.addPeerInterval(peer, bin, s, top)
		if err != nil {
			p.metrics.HistWorkerErrCounter.Inc()
			p.logger.Error(err, "histSyncWorker could not persist interval for peer, quitting...", "peer_address", peer)
			return
		}

		loggerV2.Debug("histSyncWorker pulled", "bin", bin, "start", s, "topmost", top, "peer_address", peer)
	}
}

func (p *Puller) liveSyncWorker(ctx context.Context, peer swarm.Address, bin uint8, cur uint64) {
	loggerV2 := p.logger

	defer p.wg.Done()
	p.logger.Debug("liveSyncWorker starting", "peer_address", peer, "bin", bin, "cursor", cur)
	from := cur + 1

	sleep := false

	for {
		p.metrics.LiveWorkerIterCounter.Inc()

		if sleep {
			select {
			case <-ctx.Done():
				loggerV2.Debug("liveSyncWorker context cancelled", "peer_address", peer, "bin", bin, "cursor", cur)
				return
			case <-time.After(p.syncSleepDur):
			}
			sleep = false
		}

		select {
		case <-ctx.Done():
			loggerV2.Debug("liveSyncWorker context cancelled", "peer_address", peer, "bin", bin, "cursor", cur)
			return
		default:
		}

		top, err := p.syncer.SyncInterval(ctx, peer, bin, from, pullsync.MaxCursor)
		if err != nil {
			p.metrics.LiveWorkerErrCounter.Inc()
			p.logger.Error(err, "liveSyncWorker sync error", "peer_address", peer, "bin", bin, "from", from, "topmost", top)
			sleep = true
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

		loggerV2.Debug("liveSyncWorker pulled bin", "bin", bin, "from", from, "topmost", top, "peer_address", peer)

		from = top + 1
	}
}

func (p *Puller) Close() error {
	p.logger.Info("puller shutting down")
	p.cancel()
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
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// key interval values are ALWAYS > 0
			i = intervalstore.NewIntervals(1)
			if err := p.statestore.Put(key, i); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("get peer interval: %w", err)
		}
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
	po             uint8

	cursors []uint64

	sync.Mutex
}

func newSyncPeer(addr swarm.Address, po, bins uint8) *syncPeer {
	return &syncPeer{
		address:        addr,
		po:             po,
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

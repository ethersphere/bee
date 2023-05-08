// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"context"
	random "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/blocker"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	im "github.com/ethersphere/bee/pkg/topology/kademlia/internal/metrics"
	"github.com/ethersphere/bee/pkg/topology/kademlia/internal/waitnext"
	"github.com/ethersphere/bee/pkg/topology/pslice"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/sync/errgroup"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "kademlia"

const (
	maxConnAttempts     = 1 // when there is maxConnAttempts failed connect calls for a given peer it is considered non-connectable
	maxBootNodeAttempts = 3 // how many attempts to dial to boot-nodes before giving up

	addPeerBatchSize = 500

	// To avoid context.Timeout errors during network failure, the value of
	// the peerConnectionAttemptTimeout constant must be equal to or greater
	// than 5 seconds (empirically verified).
	peerConnectionAttemptTimeout = 15 * time.Second // timeout for establishing a new connection with peer.

	flagTimeout      = 10 * time.Minute // how long before blocking a flagged peer
	blockDuration    = time.Hour        // how long to blocklist an unresponsive peer for
	blockWorkerWakup = 30 * time.Second // wake up interval for the blocker worker
)

// Default option values
const (
	defaultBitSuffixLength             = 4 // the number of bits used to create pseudo addresses for balancing
	defaultLowWaterMark                = 3 // the number of peers in consecutive deepest bins that constitute as nearest neighbours
	defaultSaturationPeers             = 8
	defaultOverSaturationPeers         = 20
	defaultBootNodeOverSaturationPeers = 20
	defaultShortRetry                  = 30 * time.Second
	defaultTimeToRetry                 = 2 * defaultShortRetry
	defaultBroadcastBinSize            = 4
	defaultPeerPingPollTime            = 5 * time.Minute  // how often to ping a peer
	defaultPingTimeout                 = 10 * time.Second // timeout for the ping response
)

var (
	errOverlayMismatch   = errors.New("overlay mismatch")
	errPruneEntry        = errors.New("prune entry")
	errEmptyBin          = errors.New("empty bin")
	errAnnounceLightNode = errors.New("announcing light node")
)

type (
	binSaturationFunc  func(bin uint8, peers, connected *pslice.PSlice, filter peerFilterFunc) bool
	sanctionedPeerFunc func(peer swarm.Address) bool
	pruneFunc          func(depth uint8)
	staticPeerFunc     func(peer swarm.Address) bool
	peerFilterFunc     func(peer swarm.Address) bool
)

var noopSanctionedPeerFn = func(_ swarm.Address) bool { return false }

// Options for injecting services to Kademlia.
type Options struct {
	SaturationFunc   binSaturationFunc
	Bootnodes        []ma.Multiaddr
	BootnodeMode     bool
	PruneFunc        pruneFunc
	StaticNodes      []swarm.Address
	ReachabilityFunc peerFilterFunc
	IgnoreRadius     bool

	BitSuffixLength             *int
	TimeToRetry                 *time.Duration
	ShortRetry                  *time.Duration
	SaturationPeers             *int
	OverSaturationPeers         *int
	BootnodeOverSaturationPeers *int
	BroadcastBinSize            *int
	LowWaterMark                *int
	PeerPingPollTime            *time.Duration
}

// kadOptions are made from Options with default values set
type kadOptions struct {
	SaturationFunc   binSaturationFunc
	Bootnodes        []ma.Multiaddr
	BootnodeMode     bool
	PruneFunc        pruneFunc
	StaticNodes      []swarm.Address
	ReachabilityFunc peerFilterFunc
	IgnoreRadius     bool

	TimeToRetry                 time.Duration
	ShortRetry                  time.Duration
	PeerPingPollTime            time.Duration
	PeerPingTimeout             time.Duration
	BitSuffixLength             int // additional depth of common prefix for bin
	SaturationPeers             int
	OverSaturationPeers         int
	BootnodeOverSaturationPeers int
	BroadcastBinSize            int
	LowWaterMark                int
}

func newKadOptions(o Options) kadOptions {
	ko := kadOptions{
		// copy values
		SaturationFunc:   o.SaturationFunc,
		Bootnodes:        o.Bootnodes,
		BootnodeMode:     o.BootnodeMode,
		PruneFunc:        o.PruneFunc,
		StaticNodes:      o.StaticNodes,
		ReachabilityFunc: o.ReachabilityFunc,
		IgnoreRadius:     o.IgnoreRadius,
		// copy or use default
		TimeToRetry:                 defaultValDuration(o.TimeToRetry, defaultTimeToRetry),
		ShortRetry:                  defaultValDuration(o.ShortRetry, defaultShortRetry),
		PeerPingPollTime:            defaultValDuration(o.PeerPingPollTime, defaultPeerPingPollTime),
		PeerPingTimeout:             defaultValDuration(o.PeerPingPollTime, defaultPingTimeout),
		BitSuffixLength:             defaultValInt(o.BitSuffixLength, defaultBitSuffixLength),
		SaturationPeers:             defaultValInt(o.SaturationPeers, defaultSaturationPeers),
		OverSaturationPeers:         defaultValInt(o.OverSaturationPeers, defaultOverSaturationPeers),
		BootnodeOverSaturationPeers: defaultValInt(o.BootnodeOverSaturationPeers, defaultBootNodeOverSaturationPeers),
		BroadcastBinSize:            defaultValInt(o.BroadcastBinSize, defaultBroadcastBinSize),
		LowWaterMark:                defaultValInt(o.LowWaterMark, defaultLowWaterMark),
	}

	if ko.SaturationFunc == nil {
		ko.SaturationFunc = makeSaturationFunc(ko)
	}

	return ko
}

func defaultValInt(v *int, d int) int {
	if v == nil {
		return d
	}
	return *v
}

func defaultValDuration(v *time.Duration, d time.Duration) time.Duration {
	if v == nil {
		return d
	}
	return *v
}

func makeSaturationFunc(o kadOptions) binSaturationFunc {
	os := o.OverSaturationPeers
	if o.BootnodeMode {
		os = o.BootnodeOverSaturationPeers
	}
	return binSaturated(os, isStaticPeer(o.StaticNodes))
}

// Kad is the Swarm forwarding kademlia implementation.
type Kad struct {
	opt               kadOptions
	base              swarm.Address         // this node's overlay address
	discovery         discovery.Driver      // the discovery driver
	addressBook       addressbook.Interface // address book to get underlays
	p2p               p2p.Service           // p2p service to connect to nodes with
	commonBinPrefixes [][]swarm.Address     // list of address prefixes for each bin
	connectedPeers    *pslice.PSlice        // a slice of peers sorted and indexed by po, indexes kept in `bins`
	knownPeers        *pslice.PSlice        // both are po aware slice of addresses
	depth             uint8                 // current neighborhood depth
	storageRadius     uint8                 // storage area of responsibility
	depthMu           sync.RWMutex          // protect depth changes
	manageC           chan struct{}         // trigger the manage forever loop to connect to new peers
	peerSig           []chan struct{}
	peerSigMtx        sync.Mutex
	logger            log.Logger // logger
	bootnode          bool       // indicates whether the node is working in bootnode mode
	collector         *im.Collector
	quit              chan struct{} // quit channel
	halt              chan struct{} // halt channel
	done              chan struct{} // signal that `manage` has quit
	wg                sync.WaitGroup
	waitNext          *waitnext.WaitNext
	metrics           metrics
	pinger            pingpong.Interface
	staticPeer        staticPeerFunc
	bgBroadcastCtx    context.Context
	bgBroadcastCancel context.CancelFunc
	blocker           *blocker.Blocker
	reachability      p2p.ReachabilityStatus
	peerFilter        peerFilterFunc
}

// New returns a new Kademlia.
func New(
	base swarm.Address,
	addressbook addressbook.Interface,
	discovery discovery.Driver,
	p2pSvc p2p.Service,
	pinger pingpong.Interface,
	metricsDB *shed.DB,
	logger log.Logger,
	o Options,
) (*Kad, error) {
	var k *Kad

	imc, err := im.NewCollector(metricsDB)
	if err != nil {
		return nil, err
	}

	opt := newKadOptions(o)

	k = &Kad{
		opt:               opt,
		base:              base,
		discovery:         discovery,
		addressBook:       addressbook,
		p2p:               p2pSvc,
		commonBinPrefixes: make([][]swarm.Address, int(swarm.MaxBins)),
		connectedPeers:    pslice.New(int(swarm.MaxBins), base),
		knownPeers:        pslice.New(int(swarm.MaxBins), base),
		manageC:           make(chan struct{}, 1),
		waitNext:          waitnext.New(),
		logger:            logger.WithName(loggerName).Register(),
		bootnode:          opt.BootnodeMode,
		collector:         imc,
		quit:              make(chan struct{}),
		halt:              make(chan struct{}),
		done:              make(chan struct{}),
		metrics:           newMetrics(),
		pinger:            pinger,
		staticPeer:        isStaticPeer(opt.StaticNodes),
		peerFilter:        opt.ReachabilityFunc,
		storageRadius:     swarm.MaxPO,
	}

	blocklistCallback := func(a swarm.Address) {
		k.logger.Debug("disconnecting peer for ping failure", "peer_address", a)
		k.metrics.Blocklist.Inc()
	}

	k.blocker = blocker.New(p2pSvc, flagTimeout, blockDuration, blockWorkerWakup, blocklistCallback, k.logger)

	if k.opt.PruneFunc == nil {
		k.opt.PruneFunc = k.pruneOversaturatedBins
	}

	if k.peerFilter == nil {
		k.peerFilter = k.collector.IsUnreachable
	}

	if k.opt.BitSuffixLength > 0 {
		k.commonBinPrefixes = generateCommonBinPrefixes(k.base, k.opt.BitSuffixLength)
	}

	k.bgBroadcastCtx, k.bgBroadcastCancel = context.WithCancel(context.Background())

	k.metrics.ReachabilityStatus.WithLabelValues(p2p.ReachabilityStatusUnknown.String()).Set(0)
	return k, nil
}

type peerConnInfo struct {
	po   uint8
	addr swarm.Address
}

// connectBalanced attempts to connect to the balanced peers first.
func (k *Kad) connectBalanced(wg *sync.WaitGroup, peerConnChan chan<- *peerConnInfo) {
	skipPeers := func(peer swarm.Address) bool {
		if k.waitNext.Waiting(peer) {
			k.metrics.TotalBeforeExpireWaits.Inc()
			return true
		}
		return false
	}

	depth := k.NeighborhoodDepth()

	for i := range k.commonBinPrefixes {

		binPeersLength := k.knownPeers.BinSize(uint8(i))

		// balancer should skip on bins where neighborhood connector would connect to peers anyway
		// and there are not enough peers in known addresses to properly balance the bin
		if i >= int(depth) && binPeersLength < len(k.commonBinPrefixes[i]) {
			continue
		}

		binPeers := k.knownPeers.BinPeers(uint8(i))
		binConnectedPeers := k.connectedPeers.BinPeers(uint8(i))

		for j := range k.commonBinPrefixes[i] {
			pseudoAddr := k.commonBinPrefixes[i][j]

			// Connect to closest known peer which we haven't tried connecting to recently.

			_, exists := nClosePeerInSlice(binConnectedPeers, pseudoAddr, noopSanctionedPeerFn, uint8(i+k.opt.BitSuffixLength+1))
			if exists {
				continue
			}

			closestKnownPeer, exists := nClosePeerInSlice(binPeers, pseudoAddr, skipPeers, uint8(i+k.opt.BitSuffixLength+1))
			if !exists {
				continue
			}

			if k.connectedPeers.Exists(closestKnownPeer) {
				continue
			}

			blocklisted, err := k.p2p.Blocklisted(closestKnownPeer)
			if err != nil {
				k.logger.Warning("peer blocklist check failed", "error", err)
			}
			if blocklisted {
				continue
			}

			wg.Add(1)
			select {
			case peerConnChan <- &peerConnInfo{
				po:   swarm.Proximity(k.base.Bytes(), closestKnownPeer.Bytes()),
				addr: closestKnownPeer,
			}:
			case <-k.quit:
				wg.Done()
				return
			}
		}
	}
}

// connectNeighbours attempts to connect to the neighbours
// which were not considered by the connectBalanced method.
func (k *Kad) connectNeighbours(wg *sync.WaitGroup, peerConnChan chan<- *peerConnInfo) {

	sent := 0
	var currentPo uint8 = 0

	_ = k.knownPeers.EachBinRev(func(addr swarm.Address, po uint8) (bool, bool, error) {

		// out of depth, skip bin
		if po < k.NeighborhoodDepth() {
			return false, true, nil
		}

		if po != currentPo {
			currentPo = po
			sent = 0
		}

		if k.connectedPeers.Exists(addr) {
			return false, false, nil
		}

		blocklisted, err := k.p2p.Blocklisted(addr)
		if err != nil {
			k.logger.Warning("peer blocklist check failed", "error", err)
		}
		if blocklisted {
			return false, false, nil
		}

		if k.waitNext.Waiting(addr) {
			k.metrics.TotalBeforeExpireWaits.Inc()
			return false, false, nil
		}

		wg.Add(1)
		select {
		case peerConnChan <- &peerConnInfo{po: po, addr: addr}:
		case <-k.quit:
			wg.Done()
			return true, false, nil
		}

		sent++

		// We want 'sent' equal to 'saturationPeers'
		// in order to skip to the next bin and speed up the topology build.
		return false, sent == k.opt.SaturationPeers, nil
	})
}

// connectionAttemptsHandler handles the connection attempts
// to peers sent by the producers to the peerConnChan.
func (k *Kad) connectionAttemptsHandler(ctx context.Context, wg *sync.WaitGroup, neighbourhoodChan, balanceChan <-chan *peerConnInfo) {
	connect := func(peer *peerConnInfo) {
		bzzAddr, err := k.addressBook.Get(peer.addr)
		switch {
		case errors.Is(err, addressbook.ErrNotFound):
			k.logger.Debug("empty address book entry for peer", "peer_address", peer.addr)
			k.knownPeers.Remove(peer.addr)
			return
		case err != nil:
			k.logger.Debug("failed to get address book entry for peer", "peer_address", peer.addr, "error", err)
			return
		}

		remove := func(peer *peerConnInfo) {
			k.waitNext.Remove(peer.addr)
			k.knownPeers.Remove(peer.addr)
			if err := k.addressBook.Remove(peer.addr); err != nil {
				k.logger.Debug("could not remove peer from addressbook", "peer_address", peer.addr)
			}
		}

		switch err = k.connect(ctx, peer.addr, bzzAddr.Underlay); {
		case errors.Is(err, p2p.ErrNetworkUnavailable):
			k.logger.Debug("network unavailable when reaching peer", "peer_overlay_address", peer.addr, "peer_underlay_address", bzzAddr.Underlay)
			return
		case errors.Is(err, errPruneEntry):
			k.logger.Debug("dial to light node", "peer_overlay_address", peer.addr, "peer_underlay_address", bzzAddr.Underlay)
			remove(peer)
			return
		case errors.Is(err, errOverlayMismatch):
			k.logger.Debug("overlay mismatch has occurred", "peer_overlay_address", peer.addr, "peer_underlay_address", bzzAddr.Underlay)
			remove(peer)
			return
		case errors.Is(err, p2p.ErrPeerBlocklisted):
			k.logger.Debug("peer still in blocklist", "peer_address", bzzAddr)
			k.logger.Warning("peer still in blocklist")
			return
		case err != nil:
			k.logger.Debug("peer not reachable from kademlia", "peer_address", bzzAddr, "error", err)
			k.logger.Warning("peer not reachable when attempting to connect")
			return
		}

		k.waitNext.Set(peer.addr, time.Now().Add(k.opt.ShortRetry), 0)

		k.connectedPeers.Add(peer.addr)

		k.metrics.TotalOutboundConnections.Inc()
		k.collector.Record(peer.addr, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionOutbound))

		k.depthMu.Lock()
		k.recalcDepth()
		k.depthMu.Unlock()

		k.logger.Info("connected to peer", "peer_address", peer.addr, "proximity_order", peer.po)
		k.notifyManageLoop()
		k.notifyPeerSig()
	}

	var (
		// The inProgress helps to avoid making a connection
		// to a peer who has the connection already in progress.
		inProgress   = make(map[string]bool)
		inProgressMu sync.Mutex
	)
	connAttempt := func(peerConnChan <-chan *peerConnInfo) {
		for {
			select {
			case <-k.quit:
				return
			case peer := <-peerConnChan:
				addr := peer.addr.String()

				if k.waitNext.Waiting(peer.addr) {
					k.metrics.TotalBeforeExpireWaits.Inc()
					wg.Done()
					continue
				}

				inProgressMu.Lock()
				if !inProgress[addr] {
					inProgress[addr] = true
					inProgressMu.Unlock()
					connect(peer)
					inProgressMu.Lock()
					delete(inProgress, addr)
				}
				inProgressMu.Unlock()
				wg.Done()
			}
		}
	}
	for i := 0; i < 32; i++ {
		go connAttempt(balanceChan)
	}
	for i := 0; i < 32; i++ {
		go connAttempt(neighbourhoodChan)
	}
}

// notifyManageLoop notifies kademlia manage loop.
func (k *Kad) notifyManageLoop() {
	select {
	case k.manageC <- struct{}{}:
	default:
	}
}

// manage is a forever loop that manages the connection to new peers
// once they get added or once others leave.
func (k *Kad) manage() {
	loggerV1 := k.logger.V(1).Register()

	defer k.wg.Done()
	defer close(k.done)
	defer k.logger.Debug("kademlia manage loop exited")

	ticker := time.NewTicker(k.opt.PeerPingPollTime)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-k.quit
		ticker.Stop()
		cancel()
	}()

	// The wg makes sure that we wait for all the connection attempts,
	// spun up by goroutines, to finish before we try the boot-nodes.
	var wg sync.WaitGroup
	neighbourhoodChan := make(chan *peerConnInfo)
	balanceChan := make(chan *peerConnInfo)
	go k.connectionAttemptsHandler(ctx, &wg, neighbourhoodChan, balanceChan)

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		for {
			select {
			case <-k.halt:
				return
			case <-k.quit:
				return
			case <-time.After(5 * time.Minute):
				start := time.Now()
				loggerV1.Debug("starting to flush metrics", "start_time", start)
				if err := k.collector.Flush(); err != nil {
					k.metrics.InternalMetricsFlushTotalErrors.Inc()
					k.logger.Debug("unable to flush metrics counters to the persistent store", "error", err)
				} else {
					k.metrics.InternalMetricsFlushTime.Observe(time.Since(start).Seconds())
					loggerV1.Debug("flush metrics done", "elapsed", time.Since(start))
				}
			}
		}
	}()

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		for {
			select {
			case <-k.halt:
				return
			case <-k.quit:
				return
			case <-ticker.C:
				k.wg.Add(1)
				go func() {
					defer k.wg.Done()
					k.recordPeerLatencies(ctx)
				}()
			}
		}
	}()

	for {
		select {
		case <-k.quit:
			return
		case <-time.After(15 * time.Second):
			k.notifyManageLoop()
		case <-k.manageC:
			start := time.Now()

			select {
			case <-k.halt:
				// halt stops dial-outs while shutting down
				return
			case <-k.quit:
				return
			default:
			}

			if k.bootnode {
				depth := k.NeighborhoodDepth()

				k.metrics.CurrentDepth.Set(float64(depth))
				k.metrics.CurrentlyKnownPeers.Set(float64(k.knownPeers.Length()))
				k.metrics.CurrentlyConnectedPeers.Set(float64(k.connectedPeers.Length()))

				continue
			}

			oldDepth := k.NeighborhoodDepth()
			k.connectBalanced(&wg, balanceChan)
			k.connectNeighbours(&wg, neighbourhoodChan)
			wg.Wait()

			depth := k.NeighborhoodDepth()

			k.opt.PruneFunc(depth)

			loggerV1.Debug("connector finished", "elapsed", time.Since(start), "old_depth", oldDepth, "new_depth", depth)

			k.metrics.CurrentDepth.Set(float64(depth))
			k.metrics.CurrentlyKnownPeers.Set(float64(k.knownPeers.Length()))
			k.metrics.CurrentlyConnectedPeers.Set(float64(k.connectedPeers.Length()))

			if k.connectedPeers.Length() == 0 {
				select {
				case <-k.halt:
					continue
				default:
				}
				k.logger.Debug("kademlia: no connected peers, trying bootnodes")
				k.connectBootNodes(ctx)
			} else {
				rs := make(map[string]float64)
				ss := k.collector.Snapshot(time.Now())

				if err := k.connectedPeers.EachBin(func(addr swarm.Address, _ uint8) (bool, bool, error) {
					if ss, ok := ss[addr.ByteString()]; ok {
						rs[ss.Reachability.String()]++
					}
					return false, false, nil
				}); err != nil {
					k.logger.Error(err, "unable to set peers reachability status")
				}

				for status, count := range rs {
					k.metrics.PeersReachabilityStatus.WithLabelValues(status).Set(count)
				}
			}
		}
	}
}

// recordPeerLatencies tries to record the average
// peer latencies from the p2p layer.
func (k *Kad) recordPeerLatencies(ctx context.Context) {
	loggerV1 := k.logger.V(1).Register()

	var wg sync.WaitGroup

	_ = k.connectedPeers.EachBin(func(addr swarm.Address, _ uint8) (bool, bool, error) {
		select {
		case <-ctx.Done():
			return false, false, nil
		case <-k.halt:
			return false, false, nil
		default:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, k.opt.PeerPingTimeout)
			defer cancel()
			switch l, err := k.pinger.Ping(ctx, addr, "ping"); {
			case err != nil:
				loggerV1.Debug("cannot get latency for peer", "peer_address", addr, "error", err)
				k.blocker.Flag(addr)
				k.metrics.Flag.Inc()
			default:
				k.blocker.Unflag(addr)
				k.metrics.Unflag.Inc()
				k.collector.Record(addr, im.PeerLatency(l))
				v := k.collector.Inspect(addr).LatencyEWMA
				k.metrics.PeerLatencyEWMA.Observe(v.Seconds())
			}
		}()
		return false, false, nil
	})
	wg.Wait()
}

// pruneOversaturatedBins disconnects out of depth peers from oversaturated bins
// while maintaining the balance of the bin and favoring peers with longers connections
func (k *Kad) pruneOversaturatedBins(depth uint8) {

	for i := range k.commonBinPrefixes {

		if i >= int(depth) {
			return
		}

		binPeersCount := k.connectedPeers.BinSize(uint8(i))
		if binPeersCount <= k.opt.OverSaturationPeers {
			continue
		}

		binPeers := k.connectedPeers.BinPeers(uint8(i))

		k.logger.Debug("starting pruning", "bin", i, "binSize", binPeersCount)

		for j := 0; j < len(k.commonBinPrefixes[i]); j++ {

			if k.connectedPeers.BinSize(uint8(i)) <= k.opt.OverSaturationPeers {
				break
			}

			pseudoAddr := k.commonBinPrefixes[i][j]
			peers := k.balancedSlotPeers(pseudoAddr, binPeers, i)

			if len(peers) <= 1 {
				continue
			}

			var smallestDuration time.Duration
			var newestPeer swarm.Address
			for _, peer := range peers {
				ss := k.collector.Inspect(peer)
				if ss == nil {
					continue
				}
				duration := ss.SessionConnectionDuration
				if smallestDuration == 0 || duration < smallestDuration {
					smallestDuration = duration
					newestPeer = peer
				}
			}
			err := k.p2p.Disconnect(newestPeer, "pruned from oversaturated bin")
			if err != nil {
				k.logger.Debug("prune disconnect failed", "error", err)
			}
		}
	}
}

func (k *Kad) balancedSlotPeers(pseudoAddr swarm.Address, peers []swarm.Address, po int) []swarm.Address {

	var ret []swarm.Address

	for _, peer := range peers {
		peerPo := swarm.ExtendedProximity(peer.Bytes(), pseudoAddr.Bytes())
		if int(peerPo) >= po+k.opt.BitSuffixLength+1 {
			ret = append(ret, peer)
		}
	}

	return ret
}

func (k *Kad) Start(_ context.Context) error {
	k.wg.Add(1)
	go k.manage()

	k.AddPeers(k.previouslyConnected()...)

	go func() {
		select {
		case <-k.halt:
			return
		case <-k.quit:
			return
		default:
		}
		var (
			start     = time.Now()
			addresses []swarm.Address
		)

		err := k.addressBook.IterateOverlays(func(addr swarm.Address) (stop bool, err error) {
			addresses = append(addresses, addr)
			if len(addresses) == addPeerBatchSize {
				k.AddPeers(addresses...)
				addresses = nil
			}
			return false, nil
		})
		if err != nil {
			k.logger.Error(err, "addressbook iterate overlays failed")
			return
		}
		k.AddPeers(addresses...)
		k.metrics.StartAddAddressBookOverlaysTime.Observe(time.Since(start).Seconds())
	}()

	// trigger the first manage loop immediately so that
	// we can start connecting to the bootnode quickly
	k.notifyManageLoop()

	return nil
}

func (k *Kad) previouslyConnected() []swarm.Address {
	loggerV1 := k.logger.V(1).Register()

	now := time.Now()
	ss := k.collector.Snapshot(now)
	loggerV1.Debug("metrics snapshot taken", "elapsed", time.Since(now))

	var peers []swarm.Address

	for addr, p := range ss {
		if p.ConnectionTotalDuration > 0 {
			peers = append(peers, swarm.NewAddress([]byte(addr)))
		}
	}

	return peers
}

func (k *Kad) connectBootNodes(ctx context.Context) {
	loggerV1 := k.logger.V(1).Register()

	var attempts, connected int
	totalAttempts := maxBootNodeAttempts * len(k.opt.Bootnodes)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, addr := range k.opt.Bootnodes {
		if attempts >= totalAttempts || connected >= 3 {
			return
		}

		if _, err := p2p.Discover(ctx, addr, func(addr ma.Multiaddr) (stop bool, err error) {
			loggerV1.Debug("connecting to bootnode", "bootnode_address", addr)
			if attempts >= maxBootNodeAttempts {
				return true, nil
			}
			bzzAddress, err := k.p2p.Connect(ctx, addr)

			attempts++
			k.metrics.TotalBootNodesConnectionAttempts.Inc()

			if err != nil {
				if !errors.Is(err, p2p.ErrAlreadyConnected) {
					k.logger.Debug("connect to bootnode failed", "bootnode_address", addr, "error", err)
					k.logger.Warning("connect to bootnode failed", "bootnode_address", addr)
					return false, err
				}
				k.logger.Debug("connect to bootnode failed", "bootnode_address", addr, "error", err)
				return false, nil
			}

			if err := k.onConnected(ctx, bzzAddress.Overlay); err != nil {
				return false, err
			}

			k.metrics.TotalOutboundConnections.Inc()
			k.collector.Record(bzzAddress.Overlay, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionOutbound))
			loggerV1.Debug("connected to bootnode", "bootnode_address", addr)
			connected++

			// connect to max 3 bootnodes
			return connected >= 3, nil
		}); err != nil && !errors.Is(err, context.Canceled) {
			k.logger.Debug("discover to bootnode failed", "bootnode_address", addr, "error", err)
			k.logger.Warning("discover to bootnode failed", "bootnode_address", addr)
			return
		}
	}
}

// binSaturated indicates whether a certain bin is saturated or not.
// when a bin is not saturated it means we would like to proactively
// initiate connections to other peers in the bin.
func binSaturated(oversaturationAmount int, staticNode staticPeerFunc) binSaturationFunc {
	return func(bin uint8, peers, connected *pslice.PSlice, filter peerFilterFunc) bool {
		size := 0
		_ = connected.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
			if po == bin && !filter(addr) && !staticNode(addr) {
				size++
			}
			return false, false, nil
		})

		return size >= oversaturationAmount
	}
}

// recalcDepth calculates, assigns the new depth, and returns if depth has changed
func (k *Kad) recalcDepth() {

	var (
		peers                 = k.connectedPeers
		filter                = k.peerFilter
		binCount              = 0
		shallowestUnsaturated = uint8(0)
		depth                 uint8
	)

	// handle edge case separately
	if peers.Length() <= k.opt.LowWaterMark {
		k.depth = 0
		return
	}

	_ = peers.EachBinRev(func(addr swarm.Address, bin uint8) (bool, bool, error) {
		if filter(addr) {
			return false, false, nil
		}
		if bin == shallowestUnsaturated {
			binCount++
			return false, false, nil
		}
		if bin > shallowestUnsaturated && binCount < k.opt.SaturationPeers {
			// this means we have less than quickSaturationPeers in the previous bin
			// therefore we can return assuming that bin is the unsaturated one.
			return true, false, nil
		}
		shallowestUnsaturated = bin
		binCount = 1

		return false, false, nil
	})
	depth = shallowestUnsaturated

	shallowestEmpty, noEmptyBins := peers.ShallowestEmpty()
	// if there are some empty bins and the shallowestEmpty is
	// smaller than the shallowestUnsaturated then set shallowest
	// unsaturated to the empty bin.
	if !noEmptyBins && shallowestEmpty < depth {
		depth = shallowestEmpty
	}

	var (
		peersCtr  = uint(0)
		candidate = uint8(0)
	)
	_ = peers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if filter(addr) {
			return false, false, nil
		}
		peersCtr++
		if peersCtr >= uint(k.opt.LowWaterMark) {
			candidate = po
			return true, false, nil
		}
		return false, false, nil
	})

	if k.storageRadius < depth && !k.opt.IgnoreRadius {
		depth = k.storageRadius
	}

	if depth > candidate {
		depth = candidate
	}

	k.depth = depth
}

// connect connects to a peer and gossips its address to our connected peers,
// as well as sends the peers we are connected to to the newly connected peer
func (k *Kad) connect(ctx context.Context, peer swarm.Address, ma ma.Multiaddr) error {
	k.logger.Debug("attempting connect to peer", "peer_address", peer)

	ctx, cancel := context.WithTimeout(ctx, peerConnectionAttemptTimeout)
	defer cancel()

	k.metrics.TotalOutboundConnectionAttempts.Inc()

	switch i, err := k.p2p.Connect(ctx, ma); {
	case errors.Is(err, p2p.ErrNetworkUnavailable):
		return err
	case k.p2p.NetworkStatus() == p2p.NetworkStatusUnavailable:
		return p2p.ErrNetworkUnavailable
	case errors.Is(err, p2p.ErrDialLightNode):
		return errPruneEntry
	case errors.Is(err, p2p.ErrAlreadyConnected):
		if !i.Overlay.Equal(peer) {
			return errOverlayMismatch
		}
		return nil
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, p2p.ErrPeerBlocklisted):
		return err
	case err != nil:
		k.logger.Debug("could not connect to peer", "peer_address", peer, "error", err)

		retryTime := time.Now().Add(k.opt.TimeToRetry)
		var e *p2p.ConnectionBackoffError
		failedAttempts := 0
		if errors.As(err, &e) {
			retryTime = e.TryAfter()
		} else {
			failedAttempts = k.waitNext.Attempts(peer)
			failedAttempts++
		}

		k.metrics.TotalOutboundConnectionFailedAttempts.Inc()
		k.collector.Record(peer, im.IncSessionConnectionRetry())

		ss := k.collector.Inspect(peer)
		quickPrune := (ss == nil || ss.HasAtMaxOneConnectionAttempt()) && isNetworkError(err)
		if (k.connectedPeers.Length() > 0 && quickPrune) || failedAttempts >= maxConnAttempts {
			k.waitNext.Remove(peer)
			k.knownPeers.Remove(peer)
			if err := k.addressBook.Remove(peer); err != nil {
				k.logger.Debug("could not remove peer from addressbook", "peer_address", peer)
			}
			k.logger.Debug("peer pruned from address book", "peer_address", peer)
		} else {
			k.waitNext.Set(peer, retryTime, failedAttempts)
		}

		return err
	case !i.Overlay.Equal(peer):
		_ = k.p2p.Disconnect(peer, errOverlayMismatch.Error())
		_ = k.p2p.Disconnect(i.Overlay, errOverlayMismatch.Error())
		return errOverlayMismatch
	}

	return k.Announce(ctx, peer, true)
}

// Announce a newly connected peer to our connected peers, but also
// notify the peer about our already connected peers
func (k *Kad) Announce(ctx context.Context, peer swarm.Address, fullnode bool) error {
	var addrs []swarm.Address

	depth := k.NeighborhoodDepth()
	isNeighbor := swarm.Proximity(peer.Bytes(), k.base.Bytes()) >= depth

outer:
	for bin := uint8(0); bin < swarm.MaxBins; bin++ {

		var (
			connectedPeers []swarm.Address
			err            error
		)

		if bin >= depth && isNeighbor {
			connectedPeers = k.binReachablePeers(bin) // broadcast all neighborhood peers
		} else {
			connectedPeers, err = randomSubset(k.binReachablePeers(bin), k.opt.BroadcastBinSize)
			if err != nil {
				return err
			}
		}

		for _, connectedPeer := range connectedPeers {
			if connectedPeer.Equal(peer) {
				continue
			}

			addrs = append(addrs, connectedPeer)

			if !fullnode {
				// we continue here so we dont gossip
				// about lightnodes to others.
				continue
			}
			// if kademlia is closing, dont enqueue anymore broadcast requests
			select {
			case <-k.bgBroadcastCtx.Done():
				// we will not interfere with the announce operation by returning here
				continue
			case <-k.halt:
				break outer
			default:
			}
			go func(connectedPeer swarm.Address) {

				// Create a new deadline ctx to prevent goroutine pile up
				cCtx, cCancel := context.WithTimeout(k.bgBroadcastCtx, time.Minute)
				defer cCancel()

				if err := k.discovery.BroadcastPeers(cCtx, connectedPeer, peer); err != nil {
					k.logger.Debug("peer gossip failed", "new_peer_address", peer, "connected_peer_address", connectedPeer, "error", err)
				}
			}(connectedPeer)
		}
	}

	if len(addrs) == 0 {
		return nil
	}

	select {
	case <-k.halt:
		return nil
	default:
	}

	err := k.discovery.BroadcastPeers(ctx, peer, addrs...)
	if err != nil {
		k.logger.Error(err, "could not broadcast to peer", "peer_address", peer)
		_ = k.p2p.Disconnect(peer, "failed broadcasting to peer")
	}

	return err
}

// AnnounceTo announces a selected peer to another.
func (k *Kad) AnnounceTo(ctx context.Context, addressee, peer swarm.Address, fullnode bool) error {
	if !fullnode {
		return errAnnounceLightNode
	}

	return k.discovery.BroadcastPeers(ctx, addressee, peer)
}

// AddPeers adds peers to the knownPeers list.
// This does not guarantee that a connection will immediately
// be made to the peer.
func (k *Kad) AddPeers(addrs ...swarm.Address) {
	k.knownPeers.Add(addrs...)
	k.notifyManageLoop()
}

func (k *Kad) Pick(peer p2p.Peer) bool {
	k.metrics.PickCalls.Inc()
	if k.bootnode || !peer.FullNode {
		// shortcircuit for bootnode mode AND light node peers - always accept connections,
		// at least until we find a better solution.
		return true
	}
	po := swarm.Proximity(k.base.Bytes(), peer.Address.Bytes())
	oversaturated := k.opt.SaturationFunc(po, k.knownPeers, k.connectedPeers, k.peerFilter)
	// pick the peer if we are not oversaturated
	if !oversaturated {
		return true
	}
	k.metrics.PickCallsFalse.Inc()
	return false
}

func (k *Kad) binReachablePeers(bin uint8) (peers []swarm.Address) {

	_ = k.EachConnectedPeerRev(func(p swarm.Address, po uint8) (bool, bool, error) {

		if po == bin {
			peers = append(peers, p)
			return false, false, nil
		}

		if po > bin {
			return true, false, nil
		}

		return false, true, nil

	}, topology.Filter{Reachable: true})

	return
}

func isStaticPeer(staticNodes []swarm.Address) func(overlay swarm.Address) bool {
	return func(overlay swarm.Address) bool {
		return swarm.ContainsAddress(staticNodes, overlay)
	}
}

// Connected is called when a peer has dialed in.
// If forceConnection is true `overSaturated` is ignored for non-bootnodes.
func (k *Kad) Connected(ctx context.Context, peer p2p.Peer, forceConnection bool) (err error) {
	defer func() {
		if err == nil {
			k.metrics.TotalInboundConnections.Inc()
			k.collector.Record(peer.Address, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionInbound))
		}
	}()

	address := peer.Address
	po := swarm.Proximity(k.base.Bytes(), address.Bytes())

	if overSaturated := k.opt.SaturationFunc(po, k.knownPeers, k.connectedPeers, k.peerFilter); overSaturated {
		if k.bootnode {
			randPeer, err := k.randomPeer(po)
			if err != nil {
				return fmt.Errorf("failed to get random peer to kick-out: %w", err)
			}
			_ = k.p2p.Disconnect(randPeer, "kicking out random peer to accommodate node")
			return k.onConnected(ctx, address)
		}
		if !forceConnection {
			return topology.ErrOversaturated
		}
	}

	return k.onConnected(ctx, address)
}

func (k *Kad) onConnected(ctx context.Context, addr swarm.Address) error {
	if err := k.Announce(ctx, addr, true); err != nil {
		return err
	}

	k.knownPeers.Add(addr)
	k.connectedPeers.Add(addr)

	k.waitNext.Remove(addr)

	k.depthMu.Lock()
	k.recalcDepth()
	k.depthMu.Unlock()

	k.notifyManageLoop()
	k.notifyPeerSig()

	return nil
}

// Disconnected is called when peer disconnects.
func (k *Kad) Disconnected(peer p2p.Peer) {
	k.logger.Info("disconnected peer", "peer_address", peer.Address)

	k.connectedPeers.Remove(peer.Address)

	k.waitNext.SetTryAfter(peer.Address, time.Now().Add(k.opt.TimeToRetry))

	k.metrics.TotalInboundDisconnections.Inc()
	k.collector.Record(peer.Address, im.PeerLogOut(time.Now()))

	k.depthMu.Lock()
	k.recalcDepth()
	k.depthMu.Unlock()

	k.notifyManageLoop()
	k.notifyPeerSig()
}

func (k *Kad) notifyPeerSig() {
	k.peerSigMtx.Lock()
	defer k.peerSigMtx.Unlock()

	for _, c := range k.peerSig {
		// Every peerSig channel has a buffer capacity of 1,
		// so every receiver will get the signal even if the
		// select statement has the default case to avoid blocking.
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

func nClosePeerInSlice(peers []swarm.Address, addr swarm.Address, spf sanctionedPeerFunc, minPO uint8) (swarm.Address, bool) {
	for _, peer := range peers {
		if spf(peer) {
			continue
		}

		if swarm.ExtendedProximity(peer.Bytes(), addr.Bytes()) >= minPO {
			return peer, true
		}
	}

	return swarm.ZeroAddress, false
}

func (k *Kad) IsReachable() bool {
	return k.reachability == p2p.ReachabilityStatusPublic
}

// ClosestPeer returns the closest peer to a given address.
func (k *Kad) ClosestPeer(addr swarm.Address, includeSelf bool, filter topology.Filter, skipPeers ...swarm.Address) (swarm.Address, error) {
	if k.connectedPeers.Length() == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}

	closest := swarm.ZeroAddress

	if includeSelf && k.reachability == p2p.ReachabilityStatusPublic {
		closest = k.base
	}

	err := k.EachConnectedPeerRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
		if swarm.ContainsAddress(skipPeers, peer) {
			return false, false, nil
		}

		if closest.IsZero() {
			closest = peer
			return false, false, nil
		}

		closer, err := peer.Closer(addr, closest)
		if closer {
			closest = peer
		}
		if err != nil {
			k.logger.Debug("closest peer", "peer", peer, "addr", addr, "error", err)
		}
		return false, false, nil
	}, filter)

	if err != nil {
		return swarm.Address{}, err
	}

	if closest.IsZero() { // no peers
		return swarm.Address{}, topology.ErrNotFound // only for light nodes
	}

	// check if self
	if closest.Equal(k.base) {
		return swarm.Address{}, topology.ErrWantSelf
	}

	return closest, nil
}

// EachConnectedPeer implements topology.PeerIterator interface.
func (k *Kad) EachConnectedPeer(f topology.EachPeerFunc, filter topology.Filter) error {
	return k.connectedPeers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if filter.Reachable && k.peerFilter(addr) {
			return false, false, nil
		}
		return f(addr, po)
	})
}

// EachConnectedPeerRev implements topology.PeerIterator interface.
func (k *Kad) EachConnectedPeerRev(f topology.EachPeerFunc, filter topology.Filter) error {
	return k.connectedPeers.EachBinRev(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if filter.Reachable && k.peerFilter(addr) {
			return false, false, nil
		}
		return f(addr, po)
	})
}
func (k *Kad) PeersCount(filter topology.Filter) int {
	return k.connectedPeers.Length()
}

// Reachable sets the peer reachability status.
func (k *Kad) Reachable(addr swarm.Address, status p2p.ReachabilityStatus) {
	loggerV1 := k.logger.V(1).Register()
	k.collector.Record(addr, im.PeerReachability(status))
	loggerV1.Debug("reachability of peer updated", "peer_address", addr, "reachability", status)
	if status == p2p.ReachabilityStatusPublic {

		k.depthMu.Lock()
		k.recalcDepth()
		k.depthMu.Unlock()

		k.notifyManageLoop()
	}
}

// UpdateReachability updates node reachability status.
// The status will be updated only once. Updates to status
// p2p.ReachabilityStatusUnknown are ignored.
func (k *Kad) UpdateReachability(status p2p.ReachabilityStatus) {
	if status == p2p.ReachabilityStatusUnknown {
		return
	}
	k.logger.Debug("reachability updated", "reachability", status)
	k.reachability = status
	k.metrics.ReachabilityStatus.WithLabelValues(status.String()).Set(0)
}

// SubscribeTopologyChange returns the channel that signals when the connected peers
// set and depth changes. Returned function is safe to be called multiple times.
func (k *Kad) SubscribeTopologyChange() (c <-chan struct{}, unsubscribe func()) {
	channel := make(chan struct{}, 1)
	var closeOnce sync.Once

	k.peerSigMtx.Lock()
	defer k.peerSigMtx.Unlock()

	k.peerSig = append(k.peerSig, channel)

	unsubscribe = func() {
		k.peerSigMtx.Lock()
		defer k.peerSigMtx.Unlock()

		for i, c := range k.peerSig {
			if c == channel {
				k.peerSig = append(k.peerSig[:i], k.peerSig[i+1:]...)
				break
			}
		}

		closeOnce.Do(func() { close(channel) })
	}

	return channel, unsubscribe
}

// NeighborhoodDepth returns the current Kademlia depth.
func (k *Kad) NeighborhoodDepth() uint8 {
	k.depthMu.RLock()
	defer k.depthMu.RUnlock()

	return k.depth
}

func (k *Kad) SetStorageRadius(d uint8) {

	k.depthMu.Lock()
	defer k.depthMu.Unlock()

	if k.storageRadius == d {
		return
	}

	k.storageRadius = d
	k.metrics.CurrentStorageDepth.Set(float64(k.storageRadius))
	k.logger.Debug("kademlia set storage radius", "radius", k.storageRadius)

	oldDepth := k.depth
	k.recalcDepth()

	if oldDepth != k.depth {
		k.notifyManageLoop()
		k.notifyPeerSig()
	}
}

func (k *Kad) Snapshot() *topology.KadParams {
	var infos []topology.BinInfo
	for i := int(swarm.MaxPO); i >= 0; i-- {
		infos = append(infos, topology.BinInfo{})
	}

	ss := k.collector.Snapshot(time.Now())

	_ = k.connectedPeers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		infos[po].BinConnected++
		infos[po].ConnectedPeers = append(
			infos[po].ConnectedPeers,
			&topology.PeerInfo{
				Address: addr,
				Metrics: createMetricsSnapshotView(ss[addr.ByteString()]),
			},
		)
		return false, false, nil
	})

	// output (k.knownPeers ¬ k.connectedPeers) here to not repeat the peers we already have in the connected peers list
	_ = k.knownPeers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		infos[po].BinPopulation++

		for _, v := range infos[po].ConnectedPeers {
			// peer already connected, don't show in the known peers list
			if v.Address.Equal(addr) {
				return false, false, nil
			}
		}

		infos[po].DisconnectedPeers = append(
			infos[po].DisconnectedPeers,
			&topology.PeerInfo{
				Address: addr,
				Metrics: createMetricsSnapshotView(ss[addr.ByteString()]),
			},
		)
		return false, false, nil
	})

	return &topology.KadParams{
		Base:                k.base.String(),
		Population:          k.knownPeers.Length(),
		Connected:           k.connectedPeers.Length(),
		Timestamp:           time.Now(),
		NNLowWatermark:      k.opt.LowWaterMark,
		Depth:               k.NeighborhoodDepth(),
		Reachability:        k.reachability.String(),
		NetworkAvailability: k.p2p.NetworkStatus().String(),
		Bins: topology.KadBins{
			Bin0:  infos[0],
			Bin1:  infos[1],
			Bin2:  infos[2],
			Bin3:  infos[3],
			Bin4:  infos[4],
			Bin5:  infos[5],
			Bin6:  infos[6],
			Bin7:  infos[7],
			Bin8:  infos[8],
			Bin9:  infos[9],
			Bin10: infos[10],
			Bin11: infos[11],
			Bin12: infos[12],
			Bin13: infos[13],
			Bin14: infos[14],
			Bin15: infos[15],
			Bin16: infos[16],
			Bin17: infos[17],
			Bin18: infos[18],
			Bin19: infos[19],
			Bin20: infos[20],
			Bin21: infos[21],
			Bin22: infos[22],
			Bin23: infos[23],
			Bin24: infos[24],
			Bin25: infos[25],
			Bin26: infos[26],
			Bin27: infos[27],
			Bin28: infos[28],
			Bin29: infos[29],
			Bin30: infos[30],
			Bin31: infos[31],
		},
	}
}

// String returns a string represenstation of Kademlia.
func (k *Kad) String() string {
	j := k.Snapshot()
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		k.logger.Error(err, "could not marshal kademlia into json")
		return ""
	}
	return string(b)
}

// Halt stops outgoing connections from happening.
// This is needed while we shut down, so that further topology
// changes do not happen while we shut down.
func (k *Kad) Halt() {
	close(k.halt)
}

// Close shuts down kademlia.
func (k *Kad) Close() error {
	k.logger.Info("kademlia shutting down")
	close(k.quit)
	_ = k.blocker.Close()
	cc := make(chan struct{})

	k.bgBroadcastCancel()

	go func() {
		k.wg.Wait()
		close(cc)
	}()

	eg := errgroup.Group{}

	errTimeout := errors.New("timeout")

	eg.Go(func() error {
		select {
		case <-cc:
		case <-time.After(peerConnectionAttemptTimeout):
			return fmt.Errorf("kademlia shutting down with running goroutines: %w", errTimeout)
		}
		return nil
	})

	eg.Go(func() error {
		select {
		case <-k.done:
		case <-time.After(time.Second * 5):
			return fmt.Errorf("kademlia manage loop did not shut down properly: %w", errTimeout)
		}
		return nil
	})

	err := eg.Wait()

	k.logger.Info("kademlia persisting peer metrics")
	start := time.Now()
	if err := k.collector.Finalize(start, false); err != nil {
		k.logger.Debug("unable to finalize open sessions", "error", err)
	}
	k.logger.Debug("metrics collector finalized", "elapsed", time.Since(start))

	return err
}

func randomSubset(addrs []swarm.Address, count int) ([]swarm.Address, error) {
	if count >= len(addrs) {
		return addrs, nil
	}

	for i := 0; i < len(addrs); i++ {
		b, err := random.Int(random.Reader, big.NewInt(int64(len(addrs))))
		if err != nil {
			return nil, err
		}
		j := int(b.Int64())
		addrs[i], addrs[j] = addrs[j], addrs[i]
	}

	return addrs[:count], nil
}

func (k *Kad) randomPeer(bin uint8) (swarm.Address, error) {
	peers := k.connectedPeers.BinPeers(bin)

	for idx := 0; idx < len(peers); {
		// do not consider protected peers
		if k.staticPeer(peers[idx]) {
			peers = append(peers[:idx], peers[idx+1:]...)
			continue
		}
		idx++
	}

	if len(peers) == 0 {
		return swarm.ZeroAddress, errEmptyBin
	}

	rndIndx, err := random.Int(random.Reader, big.NewInt(int64(len(peers))))
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return peers[rndIndx.Int64()], nil
}

// createMetricsSnapshotView creates new topology.MetricSnapshotView from the
// given metrics.Snapshot and rounds all the timestamps and durations to its
// nearest second, except for the peer latency, which is given in milliseconds.
func createMetricsSnapshotView(ss *im.Snapshot) *topology.MetricSnapshotView {
	if ss == nil {
		return nil
	}
	return &topology.MetricSnapshotView{
		LastSeenTimestamp:          time.Unix(0, ss.LastSeenTimestamp).Unix(),
		SessionConnectionRetry:     ss.SessionConnectionRetry,
		ConnectionTotalDuration:    ss.ConnectionTotalDuration.Truncate(time.Second).Seconds(),
		SessionConnectionDuration:  ss.SessionConnectionDuration.Truncate(time.Second).Seconds(),
		SessionConnectionDirection: string(ss.SessionConnectionDirection),
		LatencyEWMA:                ss.LatencyEWMA.Milliseconds(),
		Reachability:               ss.Reachability.String(),
	}
}

// isNetworkError is checking various conditions that relate to network problems.
func isNetworkError(err error) bool {
	var netOpErr *net.OpError
	if errors.As(err, &netOpErr) {
		if netOpErr.Op == "dial" {
			return true
		}
		if netOpErr.Op == "read" {
			return true
		}
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	if errors.Is(err, syscall.EPIPE) {
		return true
	}
	if errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	return false
}

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
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/blocker"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/logging"
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

const (
	maxConnAttempts        = 1 // when there is maxConnAttempts failed connect calls for a given peer it is considered non-connectable
	maxBootNodeAttempts    = 3 // how many attempts to dial to boot-nodes before giving up
	defaultBitSuffixLength = 3 // the number of bits used to create pseudo addresses for balancing

	addPeerBatchSize = 500

	peerConnectionAttemptTimeout = 5 * time.Second // timeout for establishing a new connection with peer.

	flagTimeout      = 5 * time.Minute  // how long before blocking a flagged peer
	blockDuration    = time.Hour        // how long to blocklist an unresponsive peer for
	blockWorkerWakup = time.Second * 10 // wake up interval for the blocker worker
)

var (
	nnLowWatermark              = 2 // the number of peers in consecutive deepest bins that constitute as nearest neighbours
	quickSaturationPeers        = 4
	saturationPeers             = 8
	overSaturationPeers         = 20
	bootNodeOverSaturationPeers = 20
	shortRetry                  = 30 * time.Second
	timeToRetry                 = 2 * shortRetry
	broadcastBinSize            = 4
	peerPingPollTime            = 10 * time.Second // how often to ping a peer
)

var (
	errOverlayMismatch   = errors.New("overlay mismatch")
	errPruneEntry        = errors.New("prune entry")
	errEmptyBin          = errors.New("empty bin")
	errAnnounceLightNode = errors.New("announcing light node")
)

type (
	binSaturationFunc  func(bin uint8, peers, connected *pslice.PSlice, filter peerFilterFunc) (saturated bool, oversaturated bool)
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
	BitSuffixLength  int
	PruneFunc        pruneFunc
	StaticNodes      []swarm.Address
	ReachabilityFunc peerFilterFunc
}

// Kad is the Swarm forwarding kademlia implementation.
type Kad struct {
	base              swarm.Address         // this node's overlay address
	discovery         discovery.Driver      // the discovery driver
	addressBook       addressbook.Interface // address book to get underlays
	p2p               p2p.Service           // p2p service to connect to nodes with
	saturationFunc    binSaturationFunc     // pluggable saturation function
	bitSuffixLength   int                   // additional depth of common prefix for bin
	commonBinPrefixes [][]swarm.Address     // list of address prefixes for each bin
	connectedPeers    *pslice.PSlice        // a slice of peers sorted and indexed by po, indexes kept in `bins`
	knownPeers        *pslice.PSlice        // both are po aware slice of addresses
	bootnodes         []ma.Multiaddr
	depth             uint8         // current neighborhood depth
	radius            uint8         // storage area of responsibility
	depthMu           sync.RWMutex  // protect depth changes
	manageC           chan struct{} // trigger the manage forever loop to connect to new peers
	peerSig           []chan struct{}
	peerSigMtx        sync.Mutex
	logger            logging.Logger // logger
	bootnode          bool           // indicates whether the node is working in bootnode mode
	collector         *im.Collector
	quit              chan struct{} // quit channel
	halt              chan struct{} // halt channel
	done              chan struct{} // signal that `manage` has quit
	wg                sync.WaitGroup
	waitNext          *waitnext.WaitNext
	metrics           metrics
	pruneFunc         pruneFunc // pluggable prune function
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
	p2p p2p.Service,
	pinger pingpong.Interface,
	metricsDB *shed.DB,
	logger logging.Logger,
	o Options,
) (*Kad, error) {
	if o.SaturationFunc == nil {
		os := overSaturationPeers
		if o.BootnodeMode {
			os = bootNodeOverSaturationPeers
		}
		o.SaturationFunc = binSaturated(os, isStaticPeer(o.StaticNodes))
	}
	if o.BitSuffixLength == 0 {
		o.BitSuffixLength = defaultBitSuffixLength
	}

	start := time.Now()
	imc, err := im.NewCollector(metricsDB)
	if err != nil {
		return nil, err
	}
	logger.Debugf("kademlia: NewCollector(...) took %v", time.Since(start))

	k := &Kad{
		base:              base,
		discovery:         discovery,
		addressBook:       addressbook,
		p2p:               p2p,
		saturationFunc:    o.SaturationFunc,
		bitSuffixLength:   o.BitSuffixLength,
		commonBinPrefixes: make([][]swarm.Address, int(swarm.MaxBins)),
		connectedPeers:    pslice.New(int(swarm.MaxBins), base),
		knownPeers:        pslice.New(int(swarm.MaxBins), base),
		bootnodes:         o.Bootnodes,
		manageC:           make(chan struct{}, 1),
		waitNext:          waitnext.New(),
		logger:            logger,
		bootnode:          o.BootnodeMode,
		collector:         imc,
		quit:              make(chan struct{}),
		halt:              make(chan struct{}),
		done:              make(chan struct{}),
		metrics:           newMetrics(),
		pruneFunc:         o.PruneFunc,
		pinger:            pinger,
		staticPeer:        isStaticPeer(o.StaticNodes),
		peerFilter:        o.ReachabilityFunc,
	}

	blocklistCallback := func(a swarm.Address) {
		k.logger.Debugf("kademlia: disconnecting peer %s for ping failure", a.String())
		k.metrics.Blocklist.Inc()
	}

	k.blocker = blocker.New(p2p, flagTimeout, blockDuration, blockWorkerWakup, blocklistCallback, logger)

	if k.pruneFunc == nil {
		k.pruneFunc = k.pruneOversaturatedBins
	}

	if k.peerFilter == nil {
		k.peerFilter = k.reachabilityFilter
	}

	if k.bitSuffixLength > 0 {
		k.commonBinPrefixes = generateCommonBinPrefixes(k.base, k.bitSuffixLength)
	}

	k.bgBroadcastCtx, k.bgBroadcastCancel = context.WithCancel(context.Background())

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

		for j := range k.commonBinPrefixes[i] {
			pseudoAddr := k.commonBinPrefixes[i][j]

			closestConnectedPeer, err := closestPeer(k.connectedPeers, pseudoAddr, noopSanctionedPeerFn)
			if err != nil {
				if errors.Is(err, topology.ErrNotFound) {
					break
				}
				k.logger.Errorf("closest connected peer: %v", err)
				continue
			}

			closestConnectedPO := swarm.ExtendedProximity(closestConnectedPeer.Bytes(), pseudoAddr.Bytes())
			if int(closestConnectedPO) >= i+k.bitSuffixLength+1 {
				continue
			}

			// Connect to closest known peer which we haven't tried connecting to recently.
			closestKnownPeer, err := closestPeerInSlice(binPeers, pseudoAddr, skipPeers)
			if err != nil {
				if errors.Is(err, topology.ErrNotFound) {
					break
				}
				k.logger.Errorf("closest known peer: %v", err)
				continue
			}

			if k.connectedPeers.Exists(closestKnownPeer) {
				continue
			}

			closestKnownPeerPO := swarm.ExtendedProximity(closestKnownPeer.Bytes(), pseudoAddr.Bytes())
			if int(closestKnownPeerPO) < i+k.bitSuffixLength+1 {
				continue
			}

			select {
			case <-k.quit:
				return
			default:
				wg.Add(1)
				peerConnChan <- &peerConnInfo{
					po:   swarm.Proximity(k.base.Bytes(), closestKnownPeer.Bytes()),
					addr: closestKnownPeer,
				}
			}
			break
		}
	}
}

// connectNeighbours attempts to connect to the neighbours
// which were not considered by the connectBalanced method.
func (k *Kad) connectNeighbours(wg *sync.WaitGroup, peerConnChan chan<- *peerConnInfo) {

	sent := 0
	var currentPo uint8 = 0

	_ = k.knownPeers.EachBinRev(func(addr swarm.Address, po uint8) (bool, bool, error) {
		depth := k.NeighborhoodDepth()

		// out of depth, skip bin
		if po < depth {
			return false, true, nil
		}

		if po != currentPo {
			currentPo = po
			sent = 0
		}

		if k.connectedPeers.Exists(addr) {
			return false, false, nil
		}

		if k.waitNext.Waiting(addr) {
			k.metrics.TotalBeforeExpireWaits.Inc()
			return false, false, nil
		}

		select {
		case <-k.quit:
			return true, false, nil
		default:
			wg.Add(1)
			peerConnChan <- &peerConnInfo{
				po:   po,
				addr: addr,
			}
			sent++
		}

		// We want 'sent' equal to 'saturationPeers'
		// in order to skip to the next bin and speed up the topology build.
		return false, sent == saturationPeers, nil
	})
}

// connectionAttemptsHandler handles the connection attempts
// to peers sent by the producers to the peerConnChan.
func (k *Kad) connectionAttemptsHandler(ctx context.Context, wg *sync.WaitGroup, neighbourhoodChan, balanceChan <-chan *peerConnInfo) {
	connect := func(peer *peerConnInfo) {
		bzzAddr, err := k.addressBook.Get(peer.addr)
		switch {
		case errors.Is(err, addressbook.ErrNotFound):
			k.logger.Debugf("kademlia: empty address book entry for peer %q", peer.addr)
			k.knownPeers.Remove(peer.addr)
			return
		case err != nil:
			k.logger.Debugf("kademlia: failed to get address book entry for peer %q: %v", peer.addr, err)
			return
		}

		remove := func(peer *peerConnInfo) {
			k.waitNext.Remove(peer.addr)
			k.knownPeers.Remove(peer.addr)
			if err := k.addressBook.Remove(peer.addr); err != nil {
				k.logger.Debugf("kademlia: could not remove peer %q from addressbook", peer.addr)
			}
		}

		switch err = k.connect(ctx, peer.addr, bzzAddr.Underlay); {
		case errors.Is(err, errPruneEntry):
			k.logger.Debugf("kademlia: dial to light node with overlay %q and underlay %q", peer.addr, bzzAddr.Underlay)
			remove(peer)
			return
		case errors.Is(err, errOverlayMismatch):
			k.logger.Debugf("kademlia: overlay mismatch has occurred to an overlay %q with underlay %q", peer.addr, bzzAddr.Underlay)
			remove(peer)
			return
		case err != nil:
			k.logger.Debugf("kademlia: peer not reachable from kademlia %q: %v", bzzAddr, err)
			k.logger.Warningf("peer not reachable when attempting to connect")
			return
		}

		k.waitNext.Set(peer.addr, time.Now().Add(shortRetry), 0)

		k.connectedPeers.Add(peer.addr)

		k.metrics.TotalOutboundConnections.Inc()
		k.collector.Record(peer.addr, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionOutbound))

		k.depthMu.Lock()
		k.depth = recalcDepth(k.connectedPeers, k.radius, k.peerFilter)
		k.depthMu.Unlock()

		k.logger.Debugf("kademlia: connected to peer: %q in bin: %d", peer.addr, peer.po)
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
	for i := 0; i < 16; i++ {
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
	defer k.wg.Done()
	defer close(k.done)
	defer k.logger.Debugf("kademlia manage loop exited")

	timer := time.NewTimer(0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-k.quit
		if !timer.Stop() {
			<-timer.C
		}
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
				k.logger.Tracef("kademlia: starting to flush metrics at %s", start)
				if err := k.collector.Flush(); err != nil {
					k.metrics.InternalMetricsFlushTotalErrors.Inc()
					k.logger.Debugf("kademlia: unable to flush metrics counters to the persistent store: %v", err)
				} else {
					k.metrics.InternalMetricsFlushTime.Observe(time.Since(start).Seconds())
					k.logger.Tracef("kademlia: took %s to flush", time.Since(start))
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
			case <-timer.C:
				k.wg.Add(1)
				go func() {
					defer k.wg.Done()
					k.recordPeerLatencies(ctx)
				}()
				_ = timer.Reset(peerPingPollTime)
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
				k.depthMu.Lock()
				depth := k.depth
				radius := k.radius
				k.depthMu.Unlock()

				k.metrics.CurrentDepth.Set(float64(depth))
				k.metrics.CurrentRadius.Set(float64(radius))
				k.metrics.CurrentlyKnownPeers.Set(float64(k.knownPeers.Length()))
				k.metrics.CurrentlyConnectedPeers.Set(float64(k.connectedPeers.Length()))

				continue
			}

			oldDepth := k.NeighborhoodDepth()
			k.connectBalanced(&wg, balanceChan)
			k.connectNeighbours(&wg, neighbourhoodChan)
			wg.Wait()

			k.depthMu.Lock()
			depth := k.depth
			radius := k.radius
			k.depthMu.Unlock()

			k.pruneFunc(depth)

			k.logger.Tracef(
				"kademlia: connector took %s to finish: old depth %d; new depth %d",
				time.Since(start),
				oldDepth,
				depth,
			)

			k.metrics.CurrentDepth.Set(float64(depth))
			k.metrics.CurrentRadius.Set(float64(radius))
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
			}
		}
	}
}

// recordPeerLatencies tries to record the average
// peer latencies from the p2p layer.
func (k *Kad) recordPeerLatencies(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, peerPingPollTime)
	defer cancel()
	var wg sync.WaitGroup

	_ = k.connectedPeers.EachBin(func(addr swarm.Address, _ uint8) (bool, bool, error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := k.pinger.Ping(ctx, addr, "ping")
			if err != nil {
				k.logger.Tracef("kademlia: cannot get latency for peer %s: %v", addr.String(), err)
				k.blocker.Flag(addr)
				k.metrics.Flag.Inc()
				return
			}
			k.blocker.Unflag(addr)
			k.metrics.Unflag.Inc()
			k.collector.Record(addr, im.PeerLatency(l))
			v := k.collector.Inspect(addr).LatencyEWMA
			k.metrics.PeerLatencyEWMA.Observe(v.Seconds())
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
		if binPeersCount < overSaturationPeers {
			continue
		}

		binPeers := k.connectedPeers.BinPeers(uint8(i))

		peersToRemove := binPeersCount - overSaturationPeers

		for j := 0; peersToRemove > 0 && j < len(k.commonBinPrefixes[i]); j++ {

			pseudoAddr := k.commonBinPrefixes[i][j]
			peers := k.balancedSlotPeers(pseudoAddr, binPeers, i)

			if len(peers) <= 1 {
				continue
			}

			var smallestDuration time.Duration
			var newestPeer swarm.Address
			for _, peer := range peers {
				duration := k.collector.Inspect(peer).SessionConnectionDuration
				if smallestDuration == 0 || duration < smallestDuration {
					smallestDuration = duration
					newestPeer = peer
				}
			}
			err := k.p2p.Disconnect(newestPeer, "pruned from oversaturated bin")
			if err != nil {
				k.logger.Debugf("prune disconnect fail %v", err)
			}
			peersToRemove--
		}
	}
}

func (k *Kad) balancedSlotPeers(pseudoAddr swarm.Address, peers []swarm.Address, po int) []swarm.Address {

	var ret []swarm.Address

	for _, peer := range peers {
		peerPo := swarm.ExtendedProximity(peer.Bytes(), pseudoAddr.Bytes())
		if int(peerPo) >= po+k.bitSuffixLength+1 {
			ret = append(ret, peer)
		}
	}

	return ret
}

func (k *Kad) Start(_ context.Context) error {
	k.wg.Add(1)
	go k.manage()

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
			k.logger.Errorf("addressbook overlays: %v", err)
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

func (k *Kad) connectBootNodes(ctx context.Context) {
	var attempts, connected int
	totalAttempts := maxBootNodeAttempts * len(k.bootnodes)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, addr := range k.bootnodes {
		if attempts >= totalAttempts || connected >= 3 {
			return
		}

		if _, err := p2p.Discover(ctx, addr, func(addr ma.Multiaddr) (stop bool, err error) {
			k.logger.Tracef("connecting to bootnode %s", addr)
			if attempts >= maxBootNodeAttempts {
				return true, nil
			}
			bzzAddress, err := k.p2p.Connect(ctx, addr)

			attempts++
			k.metrics.TotalBootNodesConnectionAttempts.Inc()

			if err != nil {
				if !errors.Is(err, p2p.ErrAlreadyConnected) {
					k.logger.Debugf("connect fail %s: %v", addr, err)
					k.logger.Warningf("connect to bootnode %s", addr)
					return false, err
				}
				k.logger.Debugf("connect to bootnode fail: %v", err)
				return false, nil
			}

			if err := k.onConnected(ctx, bzzAddress.Overlay); err != nil {
				return false, err
			}

			k.metrics.TotalOutboundConnections.Inc()
			k.collector.Record(bzzAddress.Overlay, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionOutbound))
			k.logger.Tracef("connected to bootnode %s", addr)
			connected++

			// connect to max 3 bootnodes
			return connected >= 3, nil
		}); err != nil && !errors.Is(err, context.Canceled) {
			k.logger.Debugf("discover fail %s: %v", addr, err)
			k.logger.Warningf("discover to bootnode %s", addr)
			return
		}
	}
}

// binSaturated indicates whether a certain bin is saturated or not.
// when a bin is not saturated it means we would like to proactively
// initiate connections to other peers in the bin.
func binSaturated(oversaturationAmount int, staticNode staticPeerFunc) binSaturationFunc {
	return func(bin uint8, peers, connected *pslice.PSlice, filter peerFilterFunc) (bool, bool) {
		potentialDepth := recalcDepth(peers, swarm.MaxPO, filter)

		// short circuit for bins which are >= depth
		if bin >= potentialDepth {
			return false, false
		}

		// lets assume for now that the minimum number of peers in a bin
		// would be 2, under which we would always want to connect to new peers
		// obviously this should be replaced with a better optimization
		// the iterator is used here since when we check if a bin is saturated,
		// the plain number of size of bin might not suffice (for example for squared
		// gaps measurement)

		size := 0
		_ = connected.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
			if !filter(addr) && po == bin && !staticNode(addr) {
				size++
			}
			return false, false, nil
		})

		return size >= saturationPeers, size >= oversaturationAmount
	}
}

func (k *Kad) reachabilityFilter(addr swarm.Address) bool {
	ss := k.collector.Inspect(addr)
	// if there is no entry yet, consider the peer as not reachable
	if ss == nil {
		return true
	}
	if ss.Reachability != p2p.ReachabilityStatusPublic {
		return true
	}
	return false
}

// recalcDepth calculates and returns the kademlia depth.
func recalcDepth(peers *pslice.PSlice, radius uint8, filter peerFilterFunc) uint8 {
	// handle edge case separately
	if peers.Length() <= nnLowWatermark {
		return 0
	}
	var (
		peersCtr                     = uint(0)
		candidate                    = uint8(0)
		shallowestEmpty, noEmptyBins = peers.ShallowestEmpty()
	)

	shallowestUnsaturated := uint8(0)
	binCount := 0
	_ = peers.EachBinRev(func(addr swarm.Address, bin uint8) (bool, bool, error) {
		if filter(addr) {
			return false, false, nil
		}
		if bin == shallowestUnsaturated {
			binCount++
			return false, false, nil
		}
		if bin > shallowestUnsaturated && binCount < quickSaturationPeers {
			// this means we have less than quickSaturationPeers in the previous bin
			// therefore we can return assuming that bin is the unsaturated one.
			return true, false, nil
		}
		shallowestUnsaturated = bin
		binCount = 1

		return false, false, nil
	})

	// if there are some empty bins and the shallowestEmpty is
	// smaller than the shallowestUnsaturated then set shallowest
	// unsaturated to the empty bin.
	if !noEmptyBins && shallowestEmpty < shallowestUnsaturated {
		shallowestUnsaturated = shallowestEmpty
	}

	_ = peers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if filter(addr) {
			return false, false, nil
		}
		peersCtr++
		if peersCtr >= uint(nnLowWatermark) {
			candidate = po
			return true, false, nil
		}
		return false, false, nil
	})
	if shallowestUnsaturated > candidate {
		if radius < candidate {
			return radius
		}
		return candidate
	}

	if radius < shallowestUnsaturated {
		return radius
	}
	return shallowestUnsaturated
}

// connect connects to a peer and gossips its address to our connected peers,
// as well as sends the peers we are connected to to the newly connected peer
func (k *Kad) connect(ctx context.Context, peer swarm.Address, ma ma.Multiaddr) error {
	k.logger.Infof("attempting to connect to peer %q", peer)

	ctx, cancel := context.WithTimeout(ctx, peerConnectionAttemptTimeout)
	defer cancel()

	k.metrics.TotalOutboundConnectionAttempts.Inc()

	switch i, err := k.p2p.Connect(ctx, ma); {
	case errors.Is(err, p2p.ErrDialLightNode):
		return errPruneEntry
	case errors.Is(err, p2p.ErrAlreadyConnected):
		if !i.Overlay.Equal(peer) {
			return errOverlayMismatch
		}
		return nil
	case errors.Is(err, context.Canceled):
		return err
	case err != nil:
		k.logger.Debugf("could not connect to peer %q: %v", peer, err)

		retryTime := time.Now().Add(timeToRetry)
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
		quickPrune := ss == nil || ss.HasAtMaxOneConnectionAttempt()
		if (k.connectedPeers.Length() > 0 && quickPrune) || failedAttempts >= maxConnAttempts {
			k.waitNext.Remove(peer)
			k.knownPeers.Remove(peer)
			if err := k.addressBook.Remove(peer); err != nil {
				k.logger.Debugf("could not remove peer from addressbook: %q", peer)
			}
			k.logger.Debugf("kademlia pruned peer from address book %q", peer)
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

	for bin := uint8(0); bin < swarm.MaxBins; bin++ {

		connectedPeers, err := randomSubset(k.binReachablePeers(bin), broadcastBinSize)
		if err != nil {
			return err
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
			default:
			}
			go func(connectedPeer swarm.Address) {

				// Create a new deadline ctx to prevent goroutine pile up
				cCtx, cCancel := context.WithTimeout(k.bgBroadcastCtx, time.Minute)
				defer cCancel()

				if err := k.discovery.BroadcastPeers(cCtx, connectedPeer, peer); err != nil {
					k.logger.Debugf("could not gossip peer %s to peer %s: %v", peer, connectedPeer, err)
				}
			}(connectedPeer)
		}
	}

	if len(addrs) == 0 {
		return nil
	}

	err := k.discovery.BroadcastPeers(ctx, peer, addrs...)
	if err != nil {
		k.logger.Errorf("kademlia: could not broadcast to peer %s", peer)
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
	if k.bootnode {
		// shortcircuit for bootnode mode - always accept connections,
		// at least until we find a better solution.
		return true
	}
	po := swarm.Proximity(k.base.Bytes(), peer.Address.Bytes())
	_, oversaturated := k.saturationFunc(po, k.knownPeers, k.connectedPeers, k.peerFilter)
	// pick the peer if we are not oversaturated
	if !oversaturated {
		return true
	}
	k.metrics.PickCallsFalse.Inc()
	return false
}

func (k *Kad) binReachablePeers(bin uint8) (peers []swarm.Address) {

	_ = k.EachPeerRev(func(p swarm.Address, po uint8) (bool, bool, error) {

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
		for _, addr := range staticNodes {
			if addr.Equal(overlay) {
				return true
			}
		}
		return false

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

	if _, overSaturated := k.saturationFunc(po, k.knownPeers, k.connectedPeers, k.peerFilter); overSaturated {
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
	k.depth = recalcDepth(k.connectedPeers, k.radius, k.peerFilter)
	k.depthMu.Unlock()

	k.notifyManageLoop()
	k.notifyPeerSig()
	return nil
}

// Disconnected is called when peer disconnects.
func (k *Kad) Disconnected(peer p2p.Peer) {
	k.logger.Debugf("kademlia: disconnected peer %s", peer.Address)

	k.connectedPeers.Remove(peer.Address)

	k.waitNext.SetTryAfter(peer.Address, time.Now().Add(timeToRetry))

	k.metrics.TotalInboundDisconnections.Inc()
	k.collector.Record(peer.Address, im.PeerLogOut(time.Now()))

	k.depthMu.Lock()
	k.depth = recalcDepth(k.connectedPeers, k.radius, k.peerFilter)
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

func closestPeer(peers *pslice.PSlice, addr swarm.Address, spf sanctionedPeerFunc) (swarm.Address, error) {
	closest := swarm.ZeroAddress
	err := peers.EachBinRev(closestPeerFunc(&closest, addr, spf))
	if err != nil {
		return closest, err
	}

	// check if found
	if closest.IsZero() {
		return closest, topology.ErrNotFound
	}

	return closest, nil
}

func closestPeerInSlice(peers []swarm.Address, addr swarm.Address, spf sanctionedPeerFunc) (swarm.Address, error) {
	closest := swarm.ZeroAddress
	closestFunc := closestPeerFunc(&closest, addr, spf)

	for _, peer := range peers {
		_, _, err := closestFunc(peer, 0)
		if err != nil {
			return closest, err
		}
	}

	// check if found
	if closest.IsZero() {
		return closest, topology.ErrNotFound
	}

	return closest, nil
}

func closestPeerFunc(closest *swarm.Address, addr swarm.Address, spf sanctionedPeerFunc) func(peer swarm.Address, po uint8) (bool, bool, error) {
	return func(peer swarm.Address, po uint8) (bool, bool, error) {
		// check whether peer is sanctioned
		if spf(peer) {
			return false, false, nil
		}
		if closest.IsZero() {
			*closest = peer
			return false, false, nil
		}

		closer, err := peer.Closer(addr, *closest)
		if err != nil {
			return false, false, err
		}
		if closer {
			*closest = peer
		}
		return false, false, nil
	}
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

	err := k.EachPeerRev(func(peer swarm.Address, po uint8) (bool, bool, error) {

		for _, a := range skipPeers {
			if a.Equal(peer) {
				return false, false, nil
			}
		}

		if closest.IsZero() {
			closest = peer
			return false, false, nil
		}

		if closer, _ := peer.Closer(addr, closest); closer {
			closest = peer
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

// IsWithinDepth returns if an address is within the neighborhood depth of a node.
func (k *Kad) IsWithinDepth(addr swarm.Address) bool {
	return swarm.Proximity(k.base.Bytes(), addr.Bytes()) >= k.NeighborhoodDepth()
}

// EachNeighbor iterates from closest bin to farthest of the neighborhood peers.
func (k *Kad) EachNeighbor(f topology.EachPeerFunc) error {
	depth := k.NeighborhoodDepth()
	fn := func(a swarm.Address, po uint8) (bool, bool, error) {
		if po < depth {
			return true, false, nil
		}
		return f(a, po)
	}
	return k.connectedPeers.EachBin(fn)
}

// EachNeighborRev iterates from farthest bin to closest of the neighborhood peers.
func (k *Kad) EachNeighborRev(f topology.EachPeerFunc) error {
	depth := k.NeighborhoodDepth()
	fn := func(a swarm.Address, po uint8) (bool, bool, error) {
		if po < depth {
			return false, true, nil
		}
		return f(a, po)
	}
	return k.connectedPeers.EachBinRev(fn)
}

// EachPeer iterates from closest bin to farthest.
func (k *Kad) EachPeer(f topology.EachPeerFunc, filter topology.Filter) error {
	return k.connectedPeers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if filter.Reachable && k.peerFilter(addr) {
			return false, false, nil
		}
		return f(addr, po)
	})
}

// EachPeerRev iterates from farthest bin to closest.
func (k *Kad) EachPeerRev(f topology.EachPeerFunc, filter topology.Filter) error {
	return k.connectedPeers.EachBinRev(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if filter.Reachable && k.peerFilter(addr) {
			return false, false, nil
		}
		return f(addr, po)
	})
}

// SetPeerReachability sets the peer reachability status.
func (k *Kad) Reachable(addr swarm.Address, status p2p.ReachabilityStatus) {
	k.collector.Record(addr, im.PeerReachability(status))
	if status == p2p.ReachabilityStatusPublic {
		k.depthMu.Lock()
		k.depth = recalcDepth(k.connectedPeers, k.radius, k.peerFilter)
		k.depthMu.Unlock()
		k.notifyManageLoop()
	}
}

// UpdateReachability updates node reachability status.
func (k *Kad) UpdateReachability(status p2p.ReachabilityStatus) {
	k.logger.Infof("kademlia: updated reachability to %s", status.String())
	k.reachability = status
}

// SubscribePeersChange returns the channel that signals when the connected peers
// set changes. Returned function is safe to be called multiple times.
func (k *Kad) SubscribePeersChange() (c <-chan struct{}, unsubscribe func()) {
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

// IsBalanced returns if Kademlia is balanced to bin.
func (k *Kad) IsBalanced(bin uint8) bool {
	k.depthMu.RLock()
	defer k.depthMu.RUnlock()

	if int(bin) > len(k.commonBinPrefixes) {
		return false
	}

	// for each pseudo address
	for i := range k.commonBinPrefixes[bin] {
		pseudoAddr := k.commonBinPrefixes[bin][i]
		closestConnectedPeer, err := closestPeer(k.connectedPeers, pseudoAddr, noopSanctionedPeerFn)
		if err != nil {
			return false
		}

		closestConnectedPO := swarm.ExtendedProximity(closestConnectedPeer.Bytes(), pseudoAddr.Bytes())
		if int(closestConnectedPO) < int(bin)+k.bitSuffixLength+1 {
			return false
		}
	}

	return true
}

func (k *Kad) SetRadius(r uint8) {
	k.depthMu.Lock()
	defer k.depthMu.Unlock()
	if k.radius == r {
		return
	}
	k.radius = r
	oldD := k.depth
	k.depth = recalcDepth(k.connectedPeers, k.radius, k.peerFilter)
	if k.depth != oldD {
		k.notifyManageLoop()
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
		Base:           k.base.String(),
		Population:     k.knownPeers.Length(),
		Connected:      k.connectedPeers.Length(),
		Timestamp:      time.Now(),
		NNLowWatermark: nnLowWatermark,
		Depth:          k.NeighborhoodDepth(),
		Reachability:   k.reachability.String(),
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
		k.logger.Errorf("could not marshal kademlia into json: %v", err)
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
			k.logger.Warning("kademlia shutting down with announce goroutines")
			return errTimeout
		}
		return nil
	})

	eg.Go(func() error {
		select {
		case <-k.done:
		case <-time.After(time.Second * 5):
			k.logger.Warning("kademlia manage loop did not shut down properly")
			return errTimeout
		}
		return nil
	})

	err := eg.Wait()

	k.logger.Info("kademlia persisting peer metrics")
	start := time.Now()
	if err := k.collector.Finalize(start, false); err != nil {
		k.logger.Debugf("kademlia: unable to finalize open sessions: %v", err)
	}
	k.logger.Debugf("kademlia: Finalize(...) took %v", time.Since(start))

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

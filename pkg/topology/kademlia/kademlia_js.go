//go:build js
// +build js

package kademlia

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/discovery"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/shed"
	"github.com/ethersphere/bee/v2/pkg/stabilization"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	im "github.com/ethersphere/bee/v2/pkg/topology/kademlia/internal/metrics"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia/internal/waitnext"
	"github.com/ethersphere/bee/v2/pkg/topology/pslice"
	"github.com/ethersphere/bee/v2/pkg/util/ioutil"
	ma "github.com/multiformats/go-multiaddr"
)

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
	staticPeer        staticPeerFunc
	bgBroadcastCtx    context.Context
	bgBroadcastCancel context.CancelFunc
	reachability      p2p.ReachabilityStatus
	detector          *stabilization.Detector
}

// New returns a new Kademlia.
func New(
	base swarm.Address,
	addressbook addressbook.Interface,
	discovery discovery.Driver,
	p2pSvc p2p.Service,
	detector *stabilization.Detector,
	logger log.Logger,
	o Options,
) (*Kad, error) {
	var k *Kad

	if o.DataDir == "" {
		logger.Warning("using in-mem store for kademlia metrics, no state will be persisted")
	} else {
		o.DataDir = filepath.Join(o.DataDir, ioutil.DataPathKademlia)
	}
	sdb, err := shed.NewDB(o.DataDir, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create metrics storage: %w", err)
	}
	imc, err := im.NewCollector(sdb)
	if err != nil {
		return nil, fmt.Errorf("unable to create metrics collector: %w", err)
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
		staticPeer:        isStaticPeer(opt.StaticNodes),
		storageRadius:     swarm.MaxPO,
		detector:          detector,
	}

	if k.opt.PruneFunc == nil {
		k.opt.PruneFunc = k.pruneOversaturatedBins
	}

	os := k.opt.OverSaturationPeers
	if k.opt.BootnodeMode {
		os = k.opt.BootnodeOverSaturationPeers
	}
	k.opt.PruneCountFunc = binPruneCount(os, isStaticPeer(k.opt.StaticNodes))

	if k.opt.ExcludeFunc == nil {
		k.opt.ExcludeFunc = func(f ...im.ExcludeOp) peerExcludeFunc {
			return func(peer swarm.Address) bool {
				return k.collector.Exclude(peer, f...)
			}
		}
	}

	if k.opt.BitSuffixLength > 0 {
		k.commonBinPrefixes = generateCommonBinPrefixes(k.base, k.opt.BitSuffixLength)
	}

	k.bgBroadcastCtx, k.bgBroadcastCancel = context.WithCancel(context.Background())

	return k, nil
}

// connectBalanced attempts to connect to the balanced peers first.
func (k *Kad) connectBalanced(wg *sync.WaitGroup, peerConnChan chan<- *peerConnInfo) {
	skipPeers := func(peer swarm.Address) bool {
		if k.waitNext.Waiting(peer) {
			return true
		}
		return false
	}

	depth := k.neighborhoodDepth()

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
		if po < k.neighborhoodDepth() {
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
			return
		case err != nil:
			k.logger.Debug("peer not reachable from kademlia", "peer_address", bzzAddr, "error", err)
			return
		}

		k.waitNext.Set(peer.addr, time.Now().Add(k.opt.ShortRetry), 0)

		k.connectedPeers.Add(peer.addr)

		k.collector.Record(peer.addr, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionOutbound))

		k.recalcDepth()

		k.logger.Debug("connected to peer", "peer_address", peer.addr, "proximity_order", peer.po)
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

	for range 32 {
		go connAttempt(balanceChan)
		go connAttempt(neighbourhoodChan)
	}
}

// manage is a forever loop that manages the connection to new peers
// once they get added or once others leave.
func (k *Kad) manage() {
	loggerV1 := k.logger.V(1).Register()

	defer k.wg.Done()
	defer close(k.done)
	defer k.logger.Debug("kademlia manage loop exited")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-k.quit
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
			case <-time.After(k.opt.PruneWakeup):
				k.opt.PruneFunc(k.neighborhoodDepth())
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
			case <-time.After(5 * time.Minute):
				start := time.Now()
				loggerV1.Debug("starting to flush metrics", "start_time", start)
				if err := k.collector.Flush(); err != nil {
					k.logger.Debug("unable to flush metrics counters to the persistent store", "error", err)
				} else {
					loggerV1.Debug("flush metrics done", "elapsed", time.Since(start))
				}
			}
		}
	}()

	// tell each neighbor about other neighbors periodically
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		for {
			select {
			case <-k.halt:
				return
			case <-k.quit:
				return
			case <-time.After(15 * time.Minute):
				var neighbors []swarm.Address
				_ = k.connectedPeers.EachBin(func(addr swarm.Address, bin uint8) (stop bool, jumpToNext bool, err error) {
					if bin < k.neighborhoodDepth() {
						return true, false, nil
					}
					neighbors = append(neighbors, addr)
					return false, false, nil
				})
				for i, peer := range neighbors {
					if err := k.discovery.BroadcastPeers(ctx, peer, append(neighbors[:i], neighbors[i+1:]...)...); err != nil {
						k.logger.Debug("broadcast neighborhood failure", "peer_address", peer, "error", err)
					}
				}
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

				continue
			}

			oldDepth := k.neighborhoodDepth()
			k.connectBalanced(&wg, balanceChan)
			k.connectNeighbours(&wg, neighbourhoodChan)
			wg.Wait()

			depth := k.neighborhoodDepth()

			loggerV1.Debug("connector finished", "elapsed", time.Since(start), "old_depth", oldDepth, "new_depth", depth)

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

			}
		}
	}
}

func (k *Kad) Start(ctx context.Context) error {
	// always discover bootnodes on startup to exclude them from protocol requests
	k.connectBootNodes(ctx)

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
	}()

	// trigger the first manage loop immediately so that
	// we can start connecting to the bootnode quickly
	k.notifyManageLoop()

	return nil
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

			k.collector.Record(bzzAddress.Overlay, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionOutbound), im.IsBootnode(true))
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

// connect connects to a peer and gossips its address to our connected peers,
// as well as sends the peers we are connected to the newly connected peer
func (k *Kad) connect(ctx context.Context, peer swarm.Address, ma ma.Multiaddr) error {
	k.logger.Debug("attempting connect to peer", "peer_address", peer)

	ctx, cancel := context.WithTimeout(ctx, peerConnectionAttemptTimeout)
	defer cancel()

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

		k.collector.Record(peer, im.IncSessionConnectionRetry())

		maxAttempts := maxConnAttempts
		if swarm.Proximity(k.base.Bytes(), peer.Bytes()) >= k.neighborhoodDepth() {
			maxAttempts = maxNeighborAttempts
		}

		if failedAttempts >= maxAttempts {
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

	k.detector.Record()

	return k.Announce(ctx, peer, true)
}

func (k *Kad) Pick(peer p2p.Peer) bool {
	if k.bootnode || !peer.FullNode {
		// shortcircuit for bootnode mode AND light node peers - always accept connections,
		// at least until we find a better solution.
		return true
	}
	po := swarm.Proximity(k.base.Bytes(), peer.Address.Bytes())
	oversaturated := k.opt.SaturationFunc(po, k.connectedPeers, k.opt.ExcludeFunc(im.Reachability(false)))
	// pick the peer if we are not oversaturated
	if !oversaturated {
		return true
	}
	return false
}

// Connected is called when a peer has dialed in.
// If forceConnection is true `overSaturated` is ignored for non-bootnodes.
func (k *Kad) Connected(ctx context.Context, peer p2p.Peer, forceConnection bool) (err error) {
	defer func() {
		if err == nil {
			k.collector.Record(peer.Address, im.PeerLogIn(time.Now(), im.PeerConnectionDirectionInbound))
		}
	}()

	address := peer.Address
	po := swarm.Proximity(k.base.Bytes(), address.Bytes())

	if overSaturated := k.opt.SaturationFunc(po, k.connectedPeers, k.opt.ExcludeFunc(im.Reachability(false))); overSaturated {
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

// Disconnected is called when peer disconnects.
func (k *Kad) Disconnected(peer p2p.Peer) {
	k.logger.Debug("disconnected peer", "peer_address", peer.Address)

	k.connectedPeers.Remove(peer.Address)

	k.waitNext.SetTryAfter(peer.Address, time.Now().Add(k.opt.TimeToRetry))

	k.collector.Record(peer.Address, im.PeerLogOut(time.Now()))

	k.recalcDepth()

	k.notifyManageLoop()
	k.notifyPeerSig()
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
}

func (k *Kad) SetStorageRadius(d uint8) {
	k.depthMu.Lock()
	defer k.depthMu.Unlock()

	if k.storageRadius == d {
		return
	}

	k.storageRadius = d
	k.logger.Debug("kademlia set storage radius", "radius", k.storageRadius)

	k.notifyManageLoop()
	k.notifyPeerSig()
}

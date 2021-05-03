// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"sync"
	"time"

	random "crypto/rand"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/pslice"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	nnLowWatermark         = 2 // the number of peers in consecutive deepest bins that constitute as nearest neighbours
	maxConnAttempts        = 3 // when there is maxConnAttempts failed connect calls for a given peer it is considered non-connectable
	maxBootnodeAttempts    = 3 // how many attempts to dial to bootnodes before giving up
	defaultBitSuffixLength = 2 // the number of bits used to create pseudo addresses for balancing
)

var (
	errMissingAddressBookEntry = errors.New("addressbook underlay entry not found")
	errOverlayMismatch         = errors.New("overlay mismatch")
	timeToRetry                = 60 * time.Second
	shortRetry                 = 30 * time.Second
	saturationPeers            = 4
	overSaturationPeers        = 16
	broadcastBinSize           = 4
)

type binSaturationFunc func(bin uint8, peers, connected *pslice.PSlice) (saturated bool, oversaturated bool)
type sanctionedPeerFunc func(peer swarm.Address) bool

var noopSanctionedPeerFn = func(_ swarm.Address) bool { return false }

// Options for injecting services to Kademlia.
type Options struct {
	SaturationFunc  binSaturationFunc
	Bootnodes       []ma.Multiaddr
	StandaloneMode  bool
	BootnodeMode    bool
	BitSuffixLength int
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
	depth             uint8                // current neighborhood depth
	radius            uint8                // storage area of responsibility
	depthMu           sync.RWMutex         // protect depth changes
	manageC           chan struct{}        // trigger the manage forever loop to connect to new peers
	waitNext          map[string]retryInfo // sanction connections to a peer, key is overlay string and value is a retry information
	waitNextMu        sync.Mutex           // synchronize map
	peerSig           []chan struct{}
	peerSigMtx        sync.Mutex
	logger            logging.Logger // logger
	standalone        bool           // indicates whether the node is working in standalone mode
	bootnode          bool           // indicates whether the node is working in bootnode mode
	quit              chan struct{}  // quit channel
	done              chan struct{}  // signal that `manage` has quit
	wg                sync.WaitGroup
}

type retryInfo struct {
	tryAfter       time.Time
	failedAttempts int
}

// New returns a new Kademlia.
func New(base swarm.Address,
	addressbook addressbook.Interface,
	discovery discovery.Driver,
	p2p p2p.Service,
	logger logging.Logger,
	o Options) *Kad {
	if o.SaturationFunc == nil {
		o.SaturationFunc = binSaturated
	}
	if o.BitSuffixLength == 0 {
		o.BitSuffixLength = defaultBitSuffixLength
	}

	k := &Kad{
		base:              base,
		discovery:         discovery,
		addressBook:       addressbook,
		p2p:               p2p,
		saturationFunc:    o.SaturationFunc,
		bitSuffixLength:   o.BitSuffixLength,
		commonBinPrefixes: make([][]swarm.Address, int(swarm.MaxBins)),
		connectedPeers:    pslice.New(int(swarm.MaxBins)),
		knownPeers:        pslice.New(int(swarm.MaxBins)),
		bootnodes:         o.Bootnodes,
		manageC:           make(chan struct{}, 1),
		waitNext:          make(map[string]retryInfo),
		logger:            logger,
		standalone:        o.StandaloneMode,
		bootnode:          o.BootnodeMode,
		quit:              make(chan struct{}),
		done:              make(chan struct{}),
		wg:                sync.WaitGroup{},
	}

	if k.bitSuffixLength > 0 {
		k.generateCommonBinPrefixes()
	}

	return k
}

func (k *Kad) generateCommonBinPrefixes() {
	bitCombinationsCount := int(math.Pow(2, float64(k.bitSuffixLength)))
	bitSufixes := make([]uint8, bitCombinationsCount)

	for i := 0; i < bitCombinationsCount; i++ {
		bitSufixes[i] = uint8(i)
	}

	addr := swarm.MustParseHexAddress(k.base.String())
	addrBytes := addr.Bytes()
	_ = addrBytes

	binPrefixes := k.commonBinPrefixes

	// copy base address
	for i := range binPrefixes {
		binPrefixes[i] = make([]swarm.Address, bitCombinationsCount)
	}

	for i := range binPrefixes {
		for j := range binPrefixes[i] {
			pseudoAddrBytes := make([]byte, len(k.base.Bytes()))
			copy(pseudoAddrBytes, k.base.Bytes())
			binPrefixes[i][j] = swarm.NewAddress(pseudoAddrBytes)
		}
	}

	for i := range binPrefixes {
		for j := range binPrefixes[i] {
			pseudoAddrBytes := binPrefixes[i][j].Bytes()

			// flip first bit for bin
			indexByte, posBit := i/8, i%8
			if hasBit(bits.Reverse8(pseudoAddrBytes[indexByte]), uint8(posBit)) {
				pseudoAddrBytes[indexByte] = bits.Reverse8(clearBit(bits.Reverse8(pseudoAddrBytes[indexByte]), uint8(posBit)))
			} else {
				pseudoAddrBytes[indexByte] = bits.Reverse8(setBit(bits.Reverse8(pseudoAddrBytes[indexByte]), uint8(posBit)))
			}

			// set pseudo suffix
			bitSuffixPos := k.bitSuffixLength - 1
			for l := i + 1; l < i+k.bitSuffixLength+1; l++ {
				index, pos := l/8, l%8

				if hasBit(bitSufixes[j], uint8(bitSuffixPos)) {
					pseudoAddrBytes[index] = bits.Reverse8(setBit(bits.Reverse8(pseudoAddrBytes[index]), uint8(pos)))
				} else {
					pseudoAddrBytes[index] = bits.Reverse8(clearBit(bits.Reverse8(pseudoAddrBytes[index]), uint8(pos)))
				}

				bitSuffixPos--
			}

			// clear rest of the bits
			for l := i + k.bitSuffixLength + 1; l < len(pseudoAddrBytes)*8; l++ {
				index, pos := l/8, l%8
				pseudoAddrBytes[index] = bits.Reverse8(clearBit(bits.Reverse8(pseudoAddrBytes[index]), uint8(pos)))
			}
		}
	}

}

// Clears the bit at pos in n.
func clearBit(n, pos uint8) uint8 {
	mask := ^(uint8(1) << pos)
	n &= mask
	return n
}

// Sets the bit at pos in the integer n.
func setBit(n, pos uint8) uint8 {
	n |= (1 << pos)
	return n
}

func hasBit(n, pos uint8) bool {
	val := n & (1 << pos)
	return (val > 0)
}

// manage is a forever loop that manages the connection to new peers
// once they get added or once others leave.
func (k *Kad) manage() {
	var peerToRemove swarm.Address

	// connect is a convenient method which wraps repetitive connect procedure..
	connect := func(ctx context.Context, peer swarm.Address, po uint8) (bool, error) {
		bzzAddr, err := k.addressBook.Get(peer)
		if err != nil {
			if err == addressbook.ErrNotFound {
				k.logger.Debugf("failed to get address book entry for peer: %q", peer)
				peerToRemove = peer
				return false, errMissingAddressBookEntry
			}
			// Either a peer is not known in the address book, in which case it
			// should be removed, or that some severe I/O problem is at hand.
			return false, err
		}

		err = k.connect(ctx, peer, bzzAddr.Underlay, po)
		switch {
		case errors.Is(err, errOverlayMismatch):
			k.knownPeers.Remove(peer, po)
			if err := k.addressBook.Remove(peer); err != nil {
				k.logger.Debugf("could not remove peer %q from addressbook", peer)
			}
			fallthrough
		case err != nil:
			k.logger.Debugf("peer not reachable from kademlia %q: %w", bzzAddr, err)
			k.logger.Warningf("peer not reachable when attempting to connect")

			k.waitNextMu.Lock()
			if _, ok := k.waitNext[peer.String()]; !ok {
				// Don't override existing data in the map.
				k.waitNext[peer.String()] = retryInfo{tryAfter: time.Now().Add(timeToRetry)}
			}
			k.waitNextMu.Unlock()
			return true, nil
		}

		k.waitNextMu.Lock()
		k.waitNext[peer.String()] = retryInfo{tryAfter: time.Now().Add(shortRetry)}
		k.waitNextMu.Unlock()

		k.connectedPeers.Add(peer, po)

		k.depthMu.Lock()
		k.depth = recalcDepth(k.connectedPeers, k.radius)
		k.depthMu.Unlock()

		return false, nil
	}

	skipPeers := func(peer swarm.Address) bool {
		k.waitNextMu.Lock()
		defer k.waitNextMu.Unlock()
		if next, ok := k.waitNext[peer.String()]; ok && time.Now().Before(next.tryAfter) {
			return true
		}
		return false
	}

	defer k.wg.Done()
	defer close(k.done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-k.quit
		cancel()
	}()

	var start time.Time
	for {
		select {
		case <-k.quit:
			return
		case <-time.After(30 * time.Second):
			// periodically try to connect to new peers
			select {
			case k.manageC <- struct{}{}:
			default:
			}
		case <-k.manageC:
			start = time.Now()
			select {
			case <-k.quit:
				return
			default:
			}
			if k.standalone {
				continue
			}

			// Attempt balanced connection first.
			err := func() error {
				for i := range k.commonBinPrefixes { // For each bin.
					for j := range k.commonBinPrefixes[i] { // And each pseudo address.
						pseudoAddr := k.commonBinPrefixes[i][j]

						closestConnectedPeer, err := closestPeer(k.connectedPeers, pseudoAddr, noopSanctionedPeerFn, swarm.ZeroAddress)
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
						closestKnownPeer, err := closestPeer(k.knownPeers, pseudoAddr, skipPeers, swarm.ZeroAddress)
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

						po := swarm.Proximity(k.base.Bytes(), closestKnownPeer.Bytes())

						next, err := connect(ctx, closestKnownPeer, po)
						if err != nil {
							return fmt.Errorf("connect error: %w", err)
						}
						if next {
							continue
						}
						k.logger.Debugf("connected to peer: %s for bin: %d", closestKnownPeer, i)

						k.notifyPeerSig()
					}
				}
				return nil
			}()
			k.logger.Tracef("kademlia balanced connector took %s to finish", time.Since(start))

			switch {
			case errors.Is(err, errMissingAddressBookEntry):
				po := swarm.Proximity(k.base.Bytes(), peerToRemove.Bytes())
				k.knownPeers.Remove(peerToRemove, po)
			case err != nil:
				k.logger.Errorf("kademlia manage loop iterator: %w", err)
			}

			err = k.knownPeers.EachBinRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
				if k.connectedPeers.Exists(peer) {
					return false, false, nil
				}

				k.waitNextMu.Lock()
				if next, ok := k.waitNext[peer.String()]; ok && time.Now().Before(next.tryAfter) {
					k.waitNextMu.Unlock()
					return false, false, nil
				}
				k.waitNextMu.Unlock()

				currentDepth := k.NeighborhoodDepth()
				if saturated, _ := k.saturationFunc(po, k.knownPeers, k.connectedPeers); saturated {
					return false, true, nil // bin is saturated, skip to next bin
				}

				next, err := connect(ctx, peer, po)
				if err != nil || next {
					return false, false, err
				}

				k.logger.Debugf("connected to peer: %s old depth: %d new depth: %d", peer, currentDepth, k.NeighborhoodDepth())

				k.notifyPeerSig()

				select {
				case <-k.quit:
					return true, false, nil
				default:
				}

				// the bin could be saturated or not, so a decision cannot
				// be made before checking the next peer, so we iterate to next
				return false, false, nil
			})
			k.logger.Tracef("kademlia iterator took %s to finish", time.Since(start))

			switch {
			case errors.Is(err, errMissingAddressBookEntry):
				po := swarm.Proximity(k.base.Bytes(), peerToRemove.Bytes())
				k.knownPeers.Remove(peerToRemove, po)
			case err != nil:
				k.logger.Errorf("kademlia manage loop iterator: %w", err)
			}

			if k.connectedPeers.Length() == 0 {
				k.logger.Debug("kademlia has no connected peers, trying bootnodes")
				k.connectBootnodes(ctx)
			}
		}
	}
}

func (k *Kad) Start(ctx context.Context) error {
	k.wg.Add(1)
	go k.manage()

	addresses, err := k.addressBook.Overlays()
	if err != nil {
		return fmt.Errorf("addressbook overlays: %w", err)
	}

	return k.AddPeers(ctx, addresses...)
}

func (k *Kad) connectBootnodes(ctx context.Context) {
	var attempts, connected int
	var totalAttempts = maxBootnodeAttempts * len(k.bootnodes)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, addr := range k.bootnodes {
		if attempts >= totalAttempts || connected >= 3 {
			return
		}

		if _, err := p2p.Discover(ctx, addr, func(addr ma.Multiaddr) (stop bool, err error) {
			k.logger.Tracef("connecting to bootnode %s", addr)
			if attempts >= maxBootnodeAttempts {
				return true, nil
			}
			bzzAddress, err := k.p2p.Connect(ctx, addr)
			attempts++
			if err != nil {
				if !errors.Is(err, p2p.ErrAlreadyConnected) {
					k.logger.Debugf("connect fail %s: %v", addr, err)
					k.logger.Warningf("connect to bootnode %s", addr)
					return false, err
				}
				k.logger.Debugf("connect to bootnode fail: %v", err)
				return false, nil
			}

			if err := k.connected(ctx, bzzAddress.Overlay); err != nil {
				return false, err
			}
			k.logger.Tracef("connected to bootnode %s", addr)
			connected++
			// connect to max 3 bootnodes
			return connected >= 3, nil
		}); err != nil {
			k.logger.Debugf("discover fail %s: %v", addr, err)
			k.logger.Warningf("discover to bootnode %s", addr)
			return
		}
	}
}

// binSaturated indicates whether a certain bin is saturated or not.
// when a bin is not saturated it means we would like to proactively
// initiate connections to other peers in the bin.
func binSaturated(bin uint8, peers, connected *pslice.PSlice) (bool, bool) {
	potentialDepth := recalcDepth(peers, swarm.MaxPO)

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
	_ = connected.EachBin(func(_ swarm.Address, po uint8) (bool, bool, error) {
		if po == bin {
			size++
		}
		return false, false, nil
	})

	return size >= saturationPeers, size >= overSaturationPeers
}

// recalcDepth calculates and returns the kademlia depth.
func recalcDepth(peers *pslice.PSlice, radius uint8) uint8 {
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
	_ = peers.EachBinRev(func(_ swarm.Address, bin uint8) (bool, bool, error) {
		if bin == shallowestUnsaturated {
			binCount++
			return false, false, nil
		}
		if bin > shallowestUnsaturated && binCount < saturationPeers {
			// this means we have less than saturationPeers in the previous bin
			// therefore we can return assuming that bin is the unsaturated one.
			return true, false, nil
		}
		// bin > shallowestUnsaturated && binCount >= saturationPeers
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

	_ = peers.EachBin(func(_ swarm.Address, po uint8) (bool, bool, error) {
		peersCtr++
		if peersCtr >= nnLowWatermark {
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
func (k *Kad) connect(ctx context.Context, peer swarm.Address, ma ma.Multiaddr, po uint8) error {
	k.logger.Infof("attempting to connect to peer %s", peer)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	i, err := k.p2p.Connect(ctx, ma)
	if err != nil {
		if errors.Is(err, p2p.ErrAlreadyConnected) {
			if !i.Overlay.Equal(peer) {
				return errOverlayMismatch
			}

			return nil
		}

		k.logger.Debugf("could not connect to peer %s: %v", peer, err)
		retryTime := time.Now().Add(timeToRetry)
		var e *p2p.ConnectionBackoffError
		k.waitNextMu.Lock()
		failedAttempts := 0
		if errors.As(err, &e) {
			retryTime = e.TryAfter()
		} else {
			info, ok := k.waitNext[peer.String()]
			if ok {
				failedAttempts = info.failedAttempts
			}

			failedAttempts++
		}

		if failedAttempts > maxConnAttempts {
			delete(k.waitNext, peer.String())
			if err := k.addressBook.Remove(peer); err != nil {
				k.logger.Debugf("could not remove peer from addressbook: %s", peer.String())
			}
			k.logger.Debugf("kademlia pruned peer from address book %s", peer.String())
		} else {
			k.waitNext[peer.String()] = retryInfo{tryAfter: retryTime, failedAttempts: failedAttempts}
		}

		k.waitNextMu.Unlock()
		return err
	}

	if !i.Overlay.Equal(peer) {
		_ = k.p2p.Disconnect(peer)
		_ = k.p2p.Disconnect(i.Overlay)
		return errOverlayMismatch
	}

	return k.Announce(ctx, peer)
}

// announce a newly connected peer to our connected peers, but also
// notify the peer about our already connected peers
func (k *Kad) Announce(ctx context.Context, peer swarm.Address) error {
	addrs := []swarm.Address{}

	for bin := uint8(0); bin < swarm.MaxBins; bin++ {

		connectedPeers, err := randomSubset(k.connectedPeers.BinPeers(bin), broadcastBinSize)
		if err != nil {
			return err
		}

		for _, connectedPeer := range connectedPeers {
			if connectedPeer.Equal(peer) {
				continue
			}

			addrs = append(addrs, connectedPeer)

			k.wg.Add(1)
			go func(connectedPeer swarm.Address) {
				defer k.wg.Done()
				if err := k.discovery.BroadcastPeers(context.Background(), connectedPeer, peer); err != nil {
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
		_ = k.p2p.Disconnect(peer)
	}

	return err
}

// AddPeers adds peers to the knownPeers list.
// This does not guarantee that a connection will immediately
// be made to the peer.
func (k *Kad) AddPeers(ctx context.Context, addrs ...swarm.Address) error {
	for _, addr := range addrs {
		if k.knownPeers.Exists(addr) {
			continue
		}

		po := swarm.Proximity(k.base.Bytes(), addr.Bytes())
		k.knownPeers.Add(addr, po)
	}

	select {
	case k.manageC <- struct{}{}:
	default:
	}

	return nil
}

func (k *Kad) Pick(peer p2p.Peer) bool {
	if k.bootnode {
		// shortcircuit for bootnode mode - always accept connections,
		// at least until we find a better solution.
		return true
	}
	po := swarm.Proximity(k.base.Bytes(), peer.Address.Bytes())
	_, oversaturated := k.saturationFunc(po, k.knownPeers, k.connectedPeers)
	// pick the peer if we are not oversaturated
	return !oversaturated
}

// Connected is called when a peer has dialed in.
func (k *Kad) Connected(ctx context.Context, peer p2p.Peer) error {
	if !k.bootnode {
		// don't run this check if we're a bootnode
		po := swarm.Proximity(k.base.Bytes(), peer.Address.Bytes())
		if _, overSaturated := k.saturationFunc(po, k.knownPeers, k.connectedPeers); overSaturated {
			return topology.ErrOversaturated
		}
	}

	if err := k.connected(ctx, peer.Address); err != nil {
		return err
	}

	select {
	case k.manageC <- struct{}{}:
	default:
	}

	return nil
}

func (k *Kad) connected(ctx context.Context, addr swarm.Address) error {
	if err := k.Announce(ctx, addr); err != nil {
		return err
	}

	po := swarm.Proximity(k.base.Bytes(), addr.Bytes())

	k.knownPeers.Add(addr, po)
	k.connectedPeers.Add(addr, po)

	k.waitNextMu.Lock()
	delete(k.waitNext, addr.String())
	k.waitNextMu.Unlock()

	k.depthMu.Lock()
	k.depth = recalcDepth(k.connectedPeers, k.radius)
	k.depthMu.Unlock()

	k.notifyPeerSig()
	return nil

}

// Disconnected is called when peer disconnects.
func (k *Kad) Disconnected(peer p2p.Peer) {
	po := swarm.Proximity(k.base.Bytes(), peer.Address.Bytes())
	k.connectedPeers.Remove(peer.Address, po)

	k.waitNextMu.Lock()
	k.waitNext[peer.Address.String()] = retryInfo{tryAfter: time.Now().Add(timeToRetry), failedAttempts: 0}
	k.waitNextMu.Unlock()

	k.depthMu.Lock()
	k.depth = recalcDepth(k.connectedPeers, k.radius)
	k.depthMu.Unlock()

	select {
	case k.manageC <- struct{}{}:
	default:
	}
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

func closestPeer(peers *pslice.PSlice, addr swarm.Address, spf sanctionedPeerFunc, skipPeers ...swarm.Address) (swarm.Address, error) {
	closest := swarm.Address{}
	err := peers.EachBinRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
		for _, a := range skipPeers {
			if a.Equal(peer) {
				return false, false, nil
			}
		}
		// check whether peer is sanctioned
		if spf(peer) {
			return false, false, nil
		}
		if closest.IsZero() {
			closest = peer
			return false, false, nil
		}
		dcmp, err := swarm.DistanceCmp(addr.Bytes(), closest.Bytes(), peer.Bytes())
		if err != nil {
			return false, false, err
		}
		switch dcmp {
		case 0:
			// do nothing
		case -1:
			// current peer is closer
			closest = peer
		case 1:
			// closest is already closer to chunk
			// do nothing
		}
		return false, false, nil
	})
	if err != nil {
		return swarm.Address{}, err
	}

	// check if found
	if closest.IsZero() {
		return swarm.Address{}, topology.ErrNotFound
	}

	return closest, nil
}

func isIn(a swarm.Address, addresses []p2p.Peer) bool {
	for _, v := range addresses {
		if v.Address.Equal(a) {
			return true
		}
	}
	return false
}

// ClosestPeer returns the closest peer to a given address.
func (k *Kad) ClosestPeer(addr swarm.Address, includeSelf bool, skipPeers ...swarm.Address) (swarm.Address, error) {
	if k.connectedPeers.Length() == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}

	peers := k.p2p.Peers()
	var peersToDisconnect []swarm.Address
	var closest = swarm.ZeroAddress

	if includeSelf {
		closest = k.base
	}

	err := k.connectedPeers.EachBinRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
		if closest.IsZero() {
			closest = peer
		}

		for _, a := range skipPeers {
			if a.Equal(peer) {
				return false, false, nil
			}
		}

		// kludge: hotfix for topology peer inconsistencies bug
		if !isIn(peer, peers) {
			a := swarm.NewAddress(peer.Bytes())
			peersToDisconnect = append(peersToDisconnect, a)
			return false, false, nil
		}

		dcmp, err := swarm.DistanceCmp(addr.Bytes(), closest.Bytes(), peer.Bytes())
		if err != nil {
			return false, false, err
		}
		switch dcmp {
		case 0:
			// do nothing
		case -1:
			// current peer is closer
			closest = peer
		case 1:
			// closest is already closer to chunk
			// do nothing
		}
		return false, false, nil
	})
	if err != nil {
		return swarm.Address{}, err
	}

	if closest.IsZero() { //no peers
		return swarm.Address{}, topology.ErrNotFound // only for light nodes
	}

	for _, v := range peersToDisconnect {
		k.Disconnected(p2p.Peer{Address: v})
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

// // EachNeighbor iterates from closest bin to farthest of the neighborhood peers.
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
func (k *Kad) EachPeer(f topology.EachPeerFunc) error {
	return k.connectedPeers.EachBin(f)
}

// EachPeerRev iterates from farthest bin to closest.
func (k *Kad) EachPeerRev(f topology.EachPeerFunc) error {
	return k.connectedPeers.EachBinRev(f)
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

	return k.neighborhoodDepth()
}

func (k *Kad) neighborhoodDepth() uint8 {
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
		closestConnectedPeer, err := closestPeer(k.connectedPeers, pseudoAddr, noopSanctionedPeerFn, swarm.ZeroAddress)
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
	k.depth = recalcDepth(k.connectedPeers, k.radius)
	if k.depth != oldD {
		select {
		case k.manageC <- struct{}{}:
		default:
		}
	}
}

func (k *Kad) Snapshot() *topology.KadParams {
	var infos []topology.BinInfo
	for i := int(swarm.MaxPO); i >= 0; i-- {
		infos = append(infos, topology.BinInfo{})
	}

	_ = k.connectedPeers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		infos[po].BinConnected++
		infos[po].ConnectedPeers = append(infos[po].ConnectedPeers, addr.String())
		return false, false, nil
	})

	// output (k.knownPeers ¬ k.connectedPeers) here to not repeat the peers we already have in the connected peers list
	_ = k.knownPeers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		infos[po].BinPopulation++

		for _, v := range infos[po].ConnectedPeers {
			// peer already connected, don't show in the known peers list
			if v == addr.String() {
				return false, false, nil
			}
		}

		infos[po].DisconnectedPeers = append(infos[po].DisconnectedPeers, addr.String())
		return false, false, nil
	})

	return &topology.KadParams{
		Base:           k.base.String(),
		Population:     k.knownPeers.Length(),
		Connected:      k.connectedPeers.Length(),
		Timestamp:      time.Now(),
		NNLowWatermark: nnLowWatermark,
		Depth:          k.NeighborhoodDepth(),
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

// Close shuts down kademlia.
func (k *Kad) Close() error {
	k.logger.Info("kademlia shutting down")
	close(k.quit)
	cc := make(chan struct{})

	go func() {
		defer close(cc)
		k.wg.Wait()
	}()

	select {
	case <-cc:
	case <-time.After(10 * time.Second):
		k.logger.Warning("kademlia shutting down with announce goroutines")
	}

	select {
	case <-k.done:
	case <-time.After(5 * time.Second):
		k.logger.Warning("kademlia manage loop did not shut down properly")
	}

	return nil
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

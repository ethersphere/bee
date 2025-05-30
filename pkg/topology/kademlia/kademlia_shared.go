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
	"math/rand"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	im "github.com/ethersphere/bee/v2/pkg/topology/kademlia/internal/metrics"
	"github.com/ethersphere/bee/v2/pkg/topology/pslice"
	ma "github.com/multiformats/go-multiaddr"
	"golang.org/x/sync/errgroup"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "kademlia"

const (
	maxConnAttempts     = 1 // when there is maxConnAttempts failed connect calls for a given peer it is considered non-connectable
	maxBootNodeAttempts = 3 // how many attempts to dial to boot-nodes before giving up
	maxNeighborAttempts = 3 // how many attempts to dial to boot-nodes before giving up

	addPeerBatchSize = 500

	// To avoid context.Timeout errors during network failure, the value of
	// the peerConnectionAttemptTimeout constant must be equal to or greater
	// than 5 seconds (empirically verified).
	peerConnectionAttemptTimeout = 15 * time.Second // timeout for establishing a new connection with peer.
)

// Default option values
const (
	defaultBitSuffixLength             = 4 // the number of bits used to create pseudo addresses for balancing, 2^4, 16 addresses
	defaultLowWaterMark                = 3 // the number of peers in consecutive deepest bins that constitute as nearest neighbours
	defaultSaturationPeers             = 8
	defaultOverSaturationPeers         = 18
	defaultBootNodeOverSaturationPeers = 20
	defaultShortRetry                  = 30 * time.Second
	defaultTimeToRetry                 = 2 * defaultShortRetry
	defaultPruneWakeup                 = 5 * time.Minute
	defaultBroadcastBinSize            = 2
)

var (
	errOverlayMismatch   = errors.New("overlay mismatch")
	errPruneEntry        = errors.New("prune entry")
	errEmptyBin          = errors.New("empty bin")
	errAnnounceLightNode = errors.New("announcing light node")
)

type (
	binSaturationFunc  func(bin uint8, connected *pslice.PSlice, exclude peerExcludeFunc) bool
	sanctionedPeerFunc func(peer swarm.Address) bool
	pruneFunc          func(depth uint8)
	pruneCountFunc     func(bin uint8, connected *pslice.PSlice, exclude peerExcludeFunc) (int, int)
	staticPeerFunc     func(peer swarm.Address) bool
	peerExcludeFunc    func(peer swarm.Address) bool
	excludeFunc        func(...im.ExcludeOp) peerExcludeFunc
)

var noopSanctionedPeerFn = func(_ swarm.Address) bool { return false }

// Options for injecting services to Kademlia.
type Options struct {
	SaturationFunc binSaturationFunc
	PruneCountFunc pruneCountFunc
	Bootnodes      []ma.Multiaddr
	BootnodeMode   bool
	PruneFunc      pruneFunc
	StaticNodes    []swarm.Address
	ExcludeFunc    excludeFunc
	DataDir        string

	BitSuffixLength             *int
	TimeToRetry                 *time.Duration
	ShortRetry                  *time.Duration
	PruneWakeup                 *time.Duration
	SaturationPeers             *int
	OverSaturationPeers         *int
	BootnodeOverSaturationPeers *int
	BroadcastBinSize            *int
	LowWaterMark                *int
}

// kadOptions are made from Options with default values set
type kadOptions struct {
	SaturationFunc binSaturationFunc
	Bootnodes      []ma.Multiaddr
	BootnodeMode   bool
	PruneCountFunc pruneCountFunc
	PruneFunc      pruneFunc
	StaticNodes    []swarm.Address
	ExcludeFunc    excludeFunc

	TimeToRetry                 time.Duration
	ShortRetry                  time.Duration
	PruneWakeup                 time.Duration
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
		SaturationFunc: o.SaturationFunc,
		Bootnodes:      o.Bootnodes,
		BootnodeMode:   o.BootnodeMode,
		PruneFunc:      o.PruneFunc,
		StaticNodes:    o.StaticNodes,
		ExcludeFunc:    o.ExcludeFunc,
		// copy or use default
		TimeToRetry:                 defaultValDuration(o.TimeToRetry, defaultTimeToRetry),
		ShortRetry:                  defaultValDuration(o.ShortRetry, defaultShortRetry),
		PruneWakeup:                 defaultValDuration(o.PruneWakeup, defaultPruneWakeup),
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

type peerConnInfo struct {
	po   uint8
	addr swarm.Address
}

// notifyManageLoop notifies kademlia manage loop.
func (k *Kad) notifyManageLoop() {
	select {
	case k.manageC <- struct{}{}:
	default:
	}
}

// pruneOversaturatedBins disconnects out of depth peers from oversaturated bins
// while maintaining the balance of the bin and favoring healthy and reachable peers.
func (k *Kad) pruneOversaturatedBins(depth uint8) {
	for i := range k.commonBinPrefixes {

		if i >= int(depth) {
			return
		}

		// skip to next bin if prune count is zero or fewer
		oldCount, pruneCount := k.opt.PruneCountFunc(uint8(i), k.connectedPeers, k.opt.ExcludeFunc(im.Reachability(false)))
		if pruneCount <= 0 {
			continue
		}

		for j := 0; j < len(k.commonBinPrefixes[i]); j++ {

			// skip to next bin if prune count is zero or fewer
			_, pruneCount := k.opt.PruneCountFunc(uint8(i), k.connectedPeers, k.opt.ExcludeFunc(im.Reachability(false)))
			if pruneCount <= 0 {
				break
			}

			binPeers := k.connectedPeers.BinPeers(uint8(i))
			peers := k.balancedSlotPeers(k.commonBinPrefixes[i][j], binPeers, i)
			if len(peers) <= 1 {
				continue
			}

			disconnectPeer := swarm.ZeroAddress
			unreachablePeer := swarm.ZeroAddress
			for _, peer := range peers {
				if ss := k.collector.Inspect(peer); ss != nil {
					if !ss.Healthy {
						disconnectPeer = peer
						break
					}
					if ss.Reachability != p2p.ReachabilityStatusPublic {
						unreachablePeer = peer
					}
				}
			}

			if disconnectPeer.IsZero() {
				if unreachablePeer.IsZero() {
					disconnectPeer = peers[rand.Intn(len(peers))]
				} else {
					disconnectPeer = unreachablePeer // pick unreachable peer
				}
			}

			err := k.p2p.Disconnect(disconnectPeer, "pruned from oversaturated bin")
			if err != nil {
				k.logger.Debug("prune disconnect failed", "error", err)
			}
		}

		newCount, _ := k.opt.PruneCountFunc(uint8(i), k.connectedPeers, k.opt.ExcludeFunc(im.Reachability(false)))

		k.logger.Debug("pruning", "bin", i, "oldBinSize", oldCount, "newBinSize", newCount)
	}
}

func (k *Kad) balancedSlotPeers(pseudoAddr swarm.Address, peers []swarm.Address, po int) []swarm.Address {
	var ret []swarm.Address

	for _, peer := range peers {
		if int(swarm.ExtendedProximity(peer.Bytes(), pseudoAddr.Bytes())) >= po+k.opt.BitSuffixLength+1 {
			ret = append(ret, peer)
		}
	}

	return ret
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

// binSaturated indicates whether a certain bin is saturated or not.
// when a bin is not saturated it means we would like to proactively
// initiate connections to other peers in the bin.
func binSaturated(oversaturationAmount int, staticNode staticPeerFunc) binSaturationFunc {
	return func(bin uint8, connected *pslice.PSlice, exclude peerExcludeFunc) bool {
		size := 0
		_ = connected.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
			if po == bin && !exclude(addr) && !staticNode(addr) {
				size++
			}
			return false, false, nil
		})

		return size >= oversaturationAmount
	}
}

// binPruneCount counts how many peers should be pruned from a bin.
func binPruneCount(oversaturationAmount int, staticNode staticPeerFunc) pruneCountFunc {
	return func(bin uint8, connected *pslice.PSlice, exclude peerExcludeFunc) (int, int) {
		size := 0
		_ = connected.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
			if po == bin && !exclude(addr) && !staticNode(addr) {
				size++
			}
			return false, false, nil
		})

		return size, size - oversaturationAmount
	}
}

// recalcDepth calculates, assigns the new depth, and returns if depth has changed
func (k *Kad) recalcDepth() {
	k.depthMu.Lock()
	defer k.depthMu.Unlock()

	var (
		peers                 = k.connectedPeers
		exclude               = k.opt.ExcludeFunc(im.Reachability(false))
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
		if exclude(addr) {
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
		if exclude(addr) {
			return false, false, nil
		}
		peersCtr++
		if peersCtr >= uint(k.opt.LowWaterMark) {
			candidate = po
			return true, false, nil
		}
		return false, false, nil
	})

	if depth > candidate {
		depth = candidate
	}

	k.depth = depth
}

// Announce a newly connected peer to our connected peers, but also
// notify the peer about our already connected peers
func (k *Kad) Announce(ctx context.Context, peer swarm.Address, fullnode bool) error {
	var addrs []swarm.Address

	depth := k.neighborhoodDepth()
	isNeighbor := swarm.Proximity(peer.Bytes(), k.base.Bytes()) >= depth

outer:
	for bin := uint8(0); bin < swarm.MaxBins; bin++ {

		var (
			connectedPeers []swarm.Address
			err            error
		)

		if bin >= depth && isNeighbor {
			connectedPeers = k.binPeers(bin, false) // broadcast all neighborhood peers
		} else {
			connectedPeers, err = randomSubset(k.binPeers(bin, true), k.opt.BroadcastBinSize)
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
				// dont gossip about lightnodes to others.
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

func (k *Kad) binPeers(bin uint8, reachable bool) (peers []swarm.Address) {
	_ = k.EachConnectedPeerRev(func(p swarm.Address, po uint8) (bool, bool, error) {
		if po == bin {
			peers = append(peers, p)
			return false, false, nil
		}

		if po > bin {
			return true, false, nil
		}

		return false, true, nil
	}, topology.Select{Reachable: reachable})

	return
}

func isStaticPeer(staticNodes []swarm.Address) func(overlay swarm.Address) bool {
	return func(overlay swarm.Address) bool {
		return swarm.ContainsAddress(staticNodes, overlay)
	}
}

func (k *Kad) onConnected(ctx context.Context, addr swarm.Address) error {
	if err := k.Announce(ctx, addr, true); err != nil {
		return err
	}

	k.knownPeers.Add(addr)
	k.connectedPeers.Add(addr)
	k.waitNext.Remove(addr)
	k.recalcDepth()
	k.notifyManageLoop()
	k.notifyPeerSig()
	k.detector.Record()

	return nil
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
func (k *Kad) ClosestPeer(addr swarm.Address, includeSelf bool, filter topology.Select, skipPeers ...swarm.Address) (swarm.Address, error) {
	if k.connectedPeers.Length() == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}

	closest := swarm.ZeroAddress

	if includeSelf && k.reachability == p2p.ReachabilityStatusPublic {
		closest = k.base
	}

	prox := swarm.Proximity(k.base.Bytes(), addr.Bytes())

	// iterate starting from bin 0 to the maximum bin
	err := k.EachConnectedPeerRev(func(peer swarm.Address, bin uint8) (bool, bool, error) {
		if swarm.ContainsAddress(skipPeers, peer) {
			return false, false, nil
		}

		if bin > prox && !closest.IsZero() {
			return true, false, nil
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
func (k *Kad) EachConnectedPeer(f topology.EachPeerFunc, filter topology.Select) error {
	excludeFunc := k.opt.ExcludeFunc(excludeFromIterator(filter)...)
	return k.connectedPeers.EachBin(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if excludeFunc(addr) {
			return false, false, nil
		}
		return f(addr, po)
	})
}

// EachConnectedPeerRev implements topology.PeerIterator interface.
func (k *Kad) EachConnectedPeerRev(f topology.EachPeerFunc, filter topology.Select) error {
	excludeFunc := k.opt.ExcludeFunc(excludeFromIterator(filter)...)
	return k.connectedPeers.EachBinRev(func(addr swarm.Address, po uint8) (bool, bool, error) {
		if excludeFunc(addr) {
			return false, false, nil
		}
		return f(addr, po)
	})
}

// Reachable sets the peer reachability status.
func (k *Kad) Reachable(addr swarm.Address, status p2p.ReachabilityStatus) {
	k.collector.Record(addr, im.PeerReachability(status))
	k.logger.Debug("reachability of peer updated", "peer_address", addr, "reachability", status)
	if status == p2p.ReachabilityStatusPublic {
		k.recalcDepth()
		k.notifyManageLoop()
	}
}

// UpdateReachability updates node reachability status.
// The status will be updated only once. Updates to status
// p2p.ReachabilityStatusUnknown are ignored.
func (k *Kad) UpdatePeerHealth(peer swarm.Address, health bool, dur time.Duration) {
	k.collector.Record(peer, im.PeerHealth(health), im.PeerLatency(dur))
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

func excludeFromIterator(filter topology.Select) []im.ExcludeOp {
	ops := make([]im.ExcludeOp, 0, 3)
	ops = append(ops, im.Bootnode())

	if filter.Reachable {
		ops = append(ops, im.Reachability(false))
	}
	if filter.Healthy {
		ops = append(ops, im.Health(false))
	}

	return ops
}

// NeighborhoodDepth returns the current Kademlia depth.
func (k *Kad) neighborhoodDepth() uint8 {
	k.depthMu.RLock()
	defer k.depthMu.RUnlock()

	return k.storageRadius
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
		Depth:               k.neighborhoodDepth(),
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
		Healthy:                    ss.Healthy,
	}
}

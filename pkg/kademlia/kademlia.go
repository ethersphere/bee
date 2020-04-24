// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery"
	"github.com/ethersphere/bee/pkg/kademlia/pslice"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	maxBins        = 16
	nnLowWatermark = 2 // the number of peers in consecutive deepest bins that constitute as nearest neighbours
)

var (
	errMissingAddressBookEntry = errors.New("addressbook underlay entry not found")
	timeToRetry                = 30 * time.Second
)

type binSaturationFunc func(bin, depth uint8, peers *pslice.PSlice) bool

// Options for injecting services to Kademlia.
type Options struct {
	Base           swarm.Address
	Discovery      discovery.Driver
	AddressBook    addressbook.Interface
	P2P            p2p.Service
	SaturationFunc binSaturationFunc
	Logger         logging.Logger
}

// Kad is the Swarm forwarding kademlia implementation.
type Kad struct {
	base           swarm.Address         // this node's overlay address
	discovery      discovery.Driver      // the discovery driver
	addressBook    addressbook.Interface // address book to get underlays
	p2p            p2p.Service           // p2p service to connect to nodes with
	saturationFunc binSaturationFunc     // pluggable saturation function
	connectedPeers *pslice.PSlice        // a slice of peers sorted and indexed by po, indexes kept in `bins`
	knownPeers     *pslice.PSlice        // both are po aware slice of addresses
	depth          uint8                 // current neighborhood depth
	depthMu        sync.RWMutex          // protect depth changes
	manageC        chan struct{}         // trigger the manage forever loop to connect to new peers
	waitNext       map[string]time.Time  // sancation connections to a peer, key is overlay string and value is time to next retry
	waitNextMu     sync.Mutex            // synchronize map
	logger         logging.Logger        // logger
	quit           chan struct{}         // quit channel
	done           chan struct{}         // signal that `manage` has quit
}

// New returns a new Kademlia.
func New(o Options) *Kad {
	if o.SaturationFunc == nil {
		o.SaturationFunc = binSaturated
	}

	k := &Kad{
		base:           o.Base,
		discovery:      o.Discovery,
		addressBook:    o.AddressBook,
		p2p:            o.P2P,
		saturationFunc: o.SaturationFunc,
		connectedPeers: pslice.New(maxBins),
		knownPeers:     pslice.New(maxBins),
		manageC:        make(chan struct{}, 1),
		waitNext:       make(map[string]time.Time),
		logger:         o.Logger,
		quit:           make(chan struct{}),
		done:           make(chan struct{}),
	}

	go k.manage()
	return k
}

// manage is a forever loop that manages the connection to new peers
// once they get added or once others leave.
func (k *Kad) manage() {
	var peerToRemove swarm.Address
	defer close(k.done)

	for {
		select {
		case <-k.quit:
			return
		case <-k.manageC:
			err := k.knownPeers.EachBinRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
				if k.connectedPeers.Exists(peer) {
					return false, false, nil
				}

				k.waitNextMu.Lock()
				if next, ok := k.waitNext[peer.String()]; ok && time.Now().Before(next) {
					k.waitNextMu.Unlock()
					return false, false, nil
				}
				k.waitNextMu.Unlock()

				currentDepth := k.NeighborhoodDepth()
				if k.saturationFunc(po, currentDepth, k.connectedPeers) {
					return false, false, nil
				}

				ma, err := k.addressBook.Get(peer)
				if err != nil {
					// either a peer is not known in the address book, in which case it
					// should be removed, or that some severe I/O problem is at hand

					if errors.Is(err, storage.ErrNotFound) {
						k.logger.Errorf("failed to get address book entry for peer: %s", peer.String())
						peerToRemove = peer
						return false, false, errMissingAddressBookEntry
					}

					return false, false, err
				}

				k.logger.Debugf("kademlia dialing to peer %s", peer.String())
				_, err = k.p2p.Connect(context.Background(), ma)
				if err != nil {
					k.logger.Debugf("error connecting to peer %s: %v", peer, err)
					k.waitNextMu.Lock()
					k.waitNext[peer.String()] = time.Now().Add(timeToRetry)
					k.waitNextMu.Unlock()

					// TODO: somehow keep track of attempts and at some point forget about the peer
					return false, false, nil // dont stop, continue to next peer
				}

				k.connectedPeers.Add(peer, po)

				k.waitNextMu.Lock()
				delete(k.waitNext, peer.String())
				k.waitNextMu.Unlock()

				k.depthMu.Lock()
				k.depth = k.recalcDepth()
				k.depthMu.Unlock()

				k.logger.Debugf("connected to peer: %s old depth: %d new depth: %d", peer, currentDepth, k.NeighborhoodDepth())

				// the bin could be saturated or not, so a decision cannot
				// be made before checking the next peer, so we iterate to next
				return false, false, nil
			})

			if err != nil {
				if errors.Is(err, errMissingAddressBookEntry) {
					po := uint8(swarm.Proximity(k.base.Bytes(), peerToRemove.Bytes()))
					k.knownPeers.Remove(peerToRemove, po)
				} else {
					k.logger.Errorf("kademlia manage loop iterator: %v", err)
				}
			}
		}
	}
}

// binSaturated indicates whether a certain bin is saturated or not.
// when a bin is not saturated it means we would like to proactively
// initiate connections to other peers in the bin.
func binSaturated(bin, depth uint8, peers *pslice.PSlice) bool {
	// short circuit for bins which are >= depth
	if bin >= depth {
		return false
	}

	// lets assume for now that the minimum number of peers in a bin
	// would be 2, under which we would always want to connect to new peers
	// obviously this should be replaced with a better optimization
	// the iterator is used here since when we check if a bin is saturated,
	// the plain number of size of bin might not suffice (for example for squared
	// gaps measurement)

	size := 0
	_ = peers.EachBin(func(_ swarm.Address, po uint8) (bool, bool, error) {
		switch {
		case po < bin:
			return true, false, nil
		case po > bin:
			return false, true, nil
		}

		size++
		return false, false, nil
	})

	return size >= 2
}

// recalcDepth returns the kademlia depth. Must be called under lock.
func (k *Kad) recalcDepth() uint8 {
	// handle edge case separately
	if k.connectedPeers.Length() <= nnLowWatermark {
		return 0
	}
	var (
		peers                        = uint(0)
		candidate                    = uint8(0)
		shallowestEmpty, noEmptyBins = k.connectedPeers.ShallowestEmpty()
	)

	_ = k.connectedPeers.EachBin(func(_ swarm.Address, po uint8) (bool, bool, error) {
		peers++
		if peers >= nnLowWatermark {
			candidate = po
			return true, false, nil
		}
		return false, false, nil
	})

	if noEmptyBins || shallowestEmpty > candidate {
		return candidate
	}

	return shallowestEmpty
}

// AddPeer adds a peer to the knownPeers list.
// This does not guarantee that a connection will immediately
// be made to the peer.
func (k *Kad) AddPeer(ctx context.Context, addr swarm.Address) error {
	if k.connectedPeers.Exists(addr) || k.knownPeers.Exists(addr) {
		return nil
	}

	po := swarm.Proximity(k.base.Bytes(), addr.Bytes())
	k.knownPeers.Add(addr, uint8(po))

	select {
	case k.manageC <- struct{}{}:
	default:
	}

	return nil
}

// Disconnected is called when peer disconnects.
func (k *Kad) Disconnected(addr swarm.Address) {
	po := uint8(swarm.Proximity(k.base.Bytes(), addr.Bytes()))
	k.connectedPeers.Remove(addr, po)

	k.waitNextMu.Lock()
	k.waitNext[addr.String()] = time.Now().Add(timeToRetry)
	k.waitNextMu.Unlock()

	k.depthMu.Lock()
	k.depth = k.recalcDepth()
	k.depthMu.Unlock()
	select {
	case k.manageC <- struct{}{}:
	default:
	}
}

// ClosestPeer returns the closest peer to a given address.
func (k *Kad) ClosestPeer(addr swarm.Address) (peerAddr swarm.Address, err error) {
	panic("not implemented") // TODO: Implement
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

// MarshalJSON returns a JSON representation of Kademlia.
func (k *Kad) MarshalJSON() ([]byte, error) {
	return k.marshal(false)
}

func (k *Kad) marshal(indent bool) ([]byte, error) {
	type binInfo struct {
		BinPopulation  uint
		BinConnected   uint
		KnownPeers     []string
		ConnectedPeers []string
	}

	type kadBins struct {
		Bin0  binInfo `json:"bin_0"`
		Bin1  binInfo `json:"bin_1"`
		Bin2  binInfo `json:"bin_2"`
		Bin3  binInfo `json:"bin_3"`
		Bin4  binInfo `json:"bin_4"`
		Bin5  binInfo `json:"bin_5"`
		Bin6  binInfo `json:"bin_6"`
		Bin7  binInfo `json:"bin_7"`
		Bin8  binInfo `json:"bin_8"`
		Bin9  binInfo `json:"bin_9"`
		Bin10 binInfo `json:"bin_10"`
		Bin11 binInfo `json:"bin_11"`
		Bin12 binInfo `json:"bin_12"`
		Bin13 binInfo `json:"bin_13"`
		Bin14 binInfo `json:"bin_14"`
		Bin15 binInfo `json:"bin_15"`
	}

	type kadParams struct {
		Base           string    // base address string
		Population     int       // known
		Connected      int       // connected count
		Timestamp      time.Time // now
		NNLowWatermark int       // low watermark for depth calculation
		Depth          uint8     // current depth
		Bins           kadBins   // individual bin info
	}

	var infos []binInfo
	for i := (maxBins - 1); i >= 0; i-- {
		infos = append(infos, binInfo{})
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

		infos[po].KnownPeers = append(infos[po].KnownPeers, addr.String())
		return false, false, nil
	})

	j := &kadParams{
		Base:           k.base.String(),
		Population:     k.knownPeers.Length(),
		Connected:      k.connectedPeers.Length(),
		Timestamp:      time.Now(),
		NNLowWatermark: nnLowWatermark,
		Depth:          k.NeighborhoodDepth(),
		Bins: kadBins{
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
		},
	}
	if indent {
		return json.MarshalIndent(j, "", "  ")
	}
	return json.Marshal(j)
}

// String returns a string represenstation of Kademlia.
func (k *Kad) String() string {
	b, err := k.marshal(true)
	if err != nil {
		k.logger.Errorf("error marshaling kademlia into json: %v", err)
		return ""
	}
	return string(b)
}

// Close shuts down kademlia.
func (k *Kad) Close() error {
	close(k.quit)
	select {
	case <-k.done:
	case <-time.After(3 * time.Second):
		k.logger.Warning("kademlia manage loop did not shut down properly")
	}
	return nil
}

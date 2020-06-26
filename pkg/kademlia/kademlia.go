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
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	maxBins         = 16
	nnLowWatermark  = 2 // the number of peers in consecutive deepest bins that constitute as nearest neighbours
	maxConnAttempts = 3 // when there is maxConnAttempts failed connect calls for a given peer it is considered non-connectable
)

var (
	errMissingAddressBookEntry = errors.New("addressbook underlay entry not found")
	errOverlayMismatch         = errors.New("overlay mismatch")
	timeToRetry                = 60 * time.Second
	shortRetry                 = 30 * time.Second
	saturationPeers            = 4
)

type binSaturationFunc func(bin uint8, peers, connected *pslice.PSlice) bool

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
	waitNext       map[string]retryInfo  // sanction connections to a peer, key is overlay string and value is a retry information
	waitNextMu     sync.Mutex            // synchronize map
	peerSig        []chan struct{}
	peerSigMtx     sync.Mutex
	logger         logging.Logger // logger
	quit           chan struct{}  // quit channel
	done           chan struct{}  // signal that `manage` has quit
	wg             sync.WaitGroup
}

type retryInfo struct {
	tryAfter       time.Time
	failedAttempts int
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
		waitNext:       make(map[string]retryInfo),
		logger:         o.Logger,
		quit:           make(chan struct{}),
		done:           make(chan struct{}),
		wg:             sync.WaitGroup{},
	}
	k.wg.Add(1)
	go k.manage()
	return k
}

// manage is a forever loop that manages the connection to new peers
// once they get added or once others leave.
func (k *Kad) manage() {
	var (
		peerToRemove swarm.Address
		start        time.Time
	)

	defer k.wg.Done()
	defer close(k.done)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-k.quit
		cancel()
	}()
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
			err := k.knownPeers.EachBinRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
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
				if saturated := k.saturationFunc(po, k.knownPeers, k.connectedPeers); saturated {
					return false, true, nil // bin is saturated, skip to next bin
				}

				bzzAddr, err := k.addressBook.Get(peer)
				if err != nil {
					if err == addressbook.ErrNotFound {
						k.logger.Debugf("failed to get address book entry for peer: %s", peer.String())
						peerToRemove = peer
						return false, false, errMissingAddressBookEntry
					}
					// either a peer is not known in the address book, in which case it
					// should be removed, or that some severe I/O problem is at hand
					return false, false, err
				}

				k.logger.Debugf("kademlia dialing to peer %s", peer.String())

				err = k.connect(ctx, peer, bzzAddr.Underlay, po)
				if err != nil {
					if errors.Is(err, errOverlayMismatch) {
						k.knownPeers.Remove(peer, po)
						if err := k.addressBook.Remove(peer); err != nil {
							k.logger.Debugf("could not remove peer from addressbook: %s", peer.String())
						}
					}
					k.logger.Debugf("error connecting to peer from kademlia %s: %v", bzzAddr.String(), err)
					k.logger.Warningf("connecting to peer %s: %v", bzzAddr.ShortString(), err)
					// continue to next
					return false, false, nil
				}

				k.waitNextMu.Lock()
				k.waitNext[peer.String()] = retryInfo{tryAfter: time.Now().Add(shortRetry)}
				k.waitNextMu.Unlock()

				k.connectedPeers.Add(peer, po)

				k.depthMu.Lock()
				k.depth = recalcDepth(k.connectedPeers)
				k.depthMu.Unlock()

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

			if err != nil {
				if errors.Is(err, errMissingAddressBookEntry) {
					po := swarm.Proximity(k.base.Bytes(), peerToRemove.Bytes())
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
func binSaturated(bin uint8, peers, connected *pslice.PSlice) bool {
	potentialDepth := recalcDepth(peers)

	// short circuit for bins which are >= depth
	if bin >= potentialDepth {
		return false
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

	return size >= saturationPeers
}

// recalcDepth calculates and returns the kademlia depth.
func recalcDepth(peers *pslice.PSlice) uint8 {
	// handle edge case separately
	if peers.Length() <= nnLowWatermark {
		return 0
	}
	var (
		peersCtr                     = uint(0)
		candidate                    = uint8(0)
		shallowestEmpty, noEmptyBins = peers.ShallowestEmpty()
	)

	_ = peers.EachBin(func(_ swarm.Address, po uint8) (bool, bool, error) {
		peersCtr++
		if peersCtr >= nnLowWatermark {
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

// connect connects to a peer and gossips its address to our connected peers,
// as well as sends the peers we are connected to to the newly connected peer
func (k *Kad) connect(ctx context.Context, peer swarm.Address, ma ma.Multiaddr, po uint8) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	i, err := k.p2p.Connect(ctx, ma)
	if err != nil {
		if errors.Is(err, p2p.ErrAlreadyConnected) {
			return nil
		}

		k.logger.Debugf("error connecting to peer %s: %v", peer, err)
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

	return k.announce(ctx, peer)
}

// announce a newly connected peer to our connected peers, but also
// notify the peer about our already connected peers
func (k *Kad) announce(ctx context.Context, peer swarm.Address) error {
	addrs := []swarm.Address{}

	_ = k.connectedPeers.EachBinRev(func(connectedPeer swarm.Address, _ uint8) (bool, bool, error) {
		if connectedPeer.Equal(peer) {
			return false, false, nil
		}

		addrs = append(addrs, connectedPeer)

		// this needs to be in a separate goroutine since a peer we are gossipping to might
		// be slow and since this function is called with the same context from kademlia connect
		// function, this might result in the unfortunate situation where we end up on
		// `err := k.discovery.BroadcastPeers(ctx, peer, addrs...)` with an already expired context
		// indicating falsely, that the peer connection has timed out.
		k.wg.Add(1)
		go func(connectedPeer swarm.Address) {
			defer k.wg.Done()
			if err := k.discovery.BroadcastPeers(context.Background(), connectedPeer, peer); err != nil {
				k.logger.Debugf("error gossiping peer %s to peer %s: %v", peer, connectedPeer, err)
			}
		}(connectedPeer)

		return false, false, nil
	})

	if len(addrs) == 0 {
		return nil
	}

	err := k.discovery.BroadcastPeers(ctx, peer, addrs...)
	if err != nil {
		_ = k.p2p.Disconnect(peer)
	}

	return err
}

// AddPeer adds a peer to the knownPeers list.
// This does not guarantee that a connection will immediately
// be made to the peer.
func (k *Kad) AddPeer(ctx context.Context, addr swarm.Address) error {
	if k.knownPeers.Exists(addr) {
		return nil
	}

	po := swarm.Proximity(k.base.Bytes(), addr.Bytes())
	k.knownPeers.Add(addr, po)

	select {
	case k.manageC <- struct{}{}:
	default:
	}

	return nil
}

// Connected is called when a peer has dialed in.
func (k *Kad) Connected(ctx context.Context, addr swarm.Address) error {
	if err := k.announce(ctx, addr); err != nil {
		return err
	}

	po := swarm.Proximity(k.base.Bytes(), addr.Bytes())
	k.knownPeers.Add(addr, po)
	k.connectedPeers.Add(addr, po)

	k.waitNextMu.Lock()
	delete(k.waitNext, addr.String())
	k.waitNextMu.Unlock()

	k.depthMu.Lock()
	k.depth = recalcDepth(k.connectedPeers)
	k.depthMu.Unlock()

	k.notifyPeerSig()

	select {
	case k.manageC <- struct{}{}:
	default:
	}

	return nil
}

// Disconnected is called when peer disconnects.
func (k *Kad) Disconnected(addr swarm.Address) {
	po := swarm.Proximity(k.base.Bytes(), addr.Bytes())
	k.connectedPeers.Remove(addr, po)

	k.waitNextMu.Lock()
	k.waitNext[addr.String()] = retryInfo{tryAfter: time.Now().Add(timeToRetry), failedAttempts: 0}
	k.waitNextMu.Unlock()

	k.depthMu.Lock()
	k.depth = recalcDepth(k.connectedPeers)
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

// ClosestPeer returns the closest peer to a given address.
func (k *Kad) ClosestPeer(addr swarm.Address) (swarm.Address, error) {
	if k.connectedPeers.Length() == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}

	closest := k.base
	err := k.connectedPeers.EachBinRev(func(peer swarm.Address, po uint8) (bool, bool, error) {
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

	// check if self
	if closest.Equal(k.base) {
		return swarm.Address{}, topology.ErrWantSelf
	}

	return closest, nil
}

// EachPeer iterates from closest bin to farthest
func (k *Kad) EachPeer(f topology.EachPeerFunc) error {
	return k.connectedPeers.EachBin(f)
}

// EachPeerRev iterates from farthest bin to closest
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

// MarshalJSON returns a JSON representation of Kademlia.
func (k *Kad) MarshalJSON() ([]byte, error) {
	return k.marshal(false)
}

func (k *Kad) marshal(indent bool) ([]byte, error) {
	type binInfo struct {
		BinPopulation     uint     `json:"population"`
		BinConnected      uint     `json:"connected"`
		DisconnectedPeers []string `json:"disconnectedPeers"`
		ConnectedPeers    []string `json:"connectedPeers"`
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
		Base           string    `json:"baseAddr"`       // base address string
		Population     int       `json:"population"`     // known
		Connected      int       `json:"connected"`      // connected count
		Timestamp      time.Time `json:"timestamp"`      // now
		NNLowWatermark int       `json:"nnLowWatermark"` // low watermark for depth calculation
		Depth          uint8     `json:"depth"`          // current depth
		Bins           kadBins   `json:"bins"`           // individual bin info
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

		infos[po].DisconnectedPeers = append(infos[po].DisconnectedPeers, addr.String())
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

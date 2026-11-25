// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	"errors"
	"slices"

	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// firstHopSource says why an origin picked a first hop.
type firstHopSource string

const (
	firstHopSourceProbe   firstHopSource = "probe"
	firstHopSourceValid   firstHopSource = "known_valid"
	firstHopSourceUnknown firstHopSource = "unknown"
	firstHopSourcePaused  firstHopSource = "paused"
)

// originPush picks the first hops of one push of an origin and records
// their results in the first-hop memory.
type originPush struct {
	ps      *PushSync
	chunk   swarm.Address
	probed  bool                      // the push made its probe
	sources map[string]firstHopSource // why each first hop was picked
}

func (ps *PushSync) newOriginPush(chunk swarm.Address) *originPush {
	return &originPush{
		ps:      ps,
		chunk:   chunk,
		sources: make(map[string]firstHopSource),
	}
}

// next picks the next first hop that is not in skip:
//  1. a probe, at most once per push and per probe interval of the
//     neighborhood: a peer closer to the chunk than every peer with a valid
//     record;
//  2. the closest peer with a valid record;
//  3. the closest peer that is not in a pause;
//  4. the peer whose pause ends first, unless the origin can store
//     the chunk itself.
func (o *originPush) next(radius uint8, skip []swarm.Address) (swarm.Address, error) {
	peer, source, err := o.pick(radius, skip)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	o.probed = o.probed || source == firstHopSourceProbe
	o.sources[peer.ByteString()] = source
	return peer, nil
}

func (o *originPush) pick(radius uint8, skip []swarm.Address) (swarm.Address, firstHopSource, error) {
	ps := o.ps
	view := ps.firstHops.View(radius, o.chunk)

	valid := o.connectedValid(radius, view.Valid)
	if known := closestOf(o.chunk, valid, skip); !known.IsZero() {
		if view.ProbeDue && !o.probed {
			candidate, err := ps.closestPeer(o.chunk, true, slices.Concat(skip, view.Paused, valid))
			if err == nil {
				if closer, _ := candidate.Closer(o.chunk, known); closer && ps.firstHops.ClaimProbe(radius, o.chunk) {
					return candidate, firstHopSourceProbe, nil
				}
			}
		}
		return known, firstHopSourceValid, nil
	}

	peer, err := ps.closestPeer(o.chunk, true, slices.Concat(skip, view.Paused))
	if !errors.Is(err, topology.ErrNotFound) || len(view.Paused) == 0 {
		return peer, firstHopSourceUnknown, err
	}
	if ps.fullNode && swarm.Proximity(ps.address.Bytes(), o.chunk.Bytes()) >= radius {
		return peer, firstHopSourceUnknown, err
	}

	// Every peer left is in a pause. Waiting would hold a push slot
	// of the pusher, so send through the connected peer whose pause ends first.
	connected := ps.connectedOf(view.Paused)
	for _, p := range view.Paused {
		if swarm.ContainsAddress(connected, p) && !swarm.ContainsAddress(skip, p) {
			return p, firstHopSourcePaused, nil
		}
	}

	return peer, firstHopSourceUnknown, err
}

// connectedValid returns the peers with a valid record that are connected,
// and removes the valid records of the others.
func (o *originPush) connectedValid(radius uint8, valid []swarm.Address) []swarm.Address {
	if len(valid) == 0 {
		return nil
	}
	connected := o.ps.connectedOf(valid)
	for _, p := range valid {
		if !swarm.ContainsAddress(connected, p) {
			o.ps.firstHops.Forget(radius, o.chunk, p)
		}
	}
	return connected
}

// closestOf returns the peer closest to the chunk among the peers that are
// not in skip, or the zero address.
func closestOf(chunk swarm.Address, peers, skip []swarm.Address) swarm.Address {
	closest := swarm.ZeroAddress
	for _, p := range peers {
		if swarm.ContainsAddress(skip, p) {
			continue
		}
		if closest.IsZero() {
			closest = p
			continue
		}
		if closer, _ := p.Closer(chunk, closest); closer {
			closest = p
		}
	}
	return closest
}

// record stores the result of a push to a first hop: a valid receipt (err is
// nil), a shallow receipt, or an error. An error starts a pause, as a shallow
// receipt does. The error of a first hop that was picked for its valid record
// only removes the record, because a single error can be accidental.
func (o *originPush) record(radius uint8, peer swarm.Address, err error) {
	source := o.sources[peer.ByteString()]
	result := "error"
	switch {
	case err == nil:
		result = "valid"
		o.ps.firstHops.Valid(radius, o.chunk, peer)
	case errors.Is(err, ErrShallowReceipt):
		result = "shallow"
		o.ps.firstHops.Pause(radius, o.chunk, peer)
	case source == firstHopSourceValid:
		o.ps.firstHops.Forget(radius, o.chunk, peer)
	default:
		o.ps.firstHops.Pause(radius, o.chunk, peer)
	}
	o.ps.metrics.FirstHop.WithLabelValues(string(source), result).Inc()
}

// connectedOf returns the peers that are connected, in one pass over the
// connected peers.
func (ps *PushSync) connectedOf(peers []swarm.Address) []swarm.Address {
	var connected []swarm.Address
	_ = ps.topologyDriver.EachConnectedPeer(func(p swarm.Address, _ uint8) (bool, bool, error) {
		if swarm.ContainsAddress(peers, p) {
			connected = append(connected, p)
		}
		return len(connected) == len(peers), false, nil
	}, topology.Select{})
	return connected
}

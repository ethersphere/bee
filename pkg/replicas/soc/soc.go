// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package replicas implements a scheme to replicate SOC chunks
// in such a way that
// - the replicas are optimally dispersed to aid cross-neighbourhood redundancy
// - the replicas addresses can be deduced by retrievers only knowing the address
// of the original content addressed chunk
// - no new chunk validation rules are introduced
package soc

import (
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// replicator running the find for replicas
type socReplicator struct {
	addr   []byte          // chunk address
	queue  [16]*socReplica // to sort addresses according to di
	exist  [30]bool        //  maps the 16 distinct nibbles on all levels
	sizes  [5]int          // number of distinct neighnourhoods redcorded for each depth
	C      chan *socReplica
	rLevel redundancy.Level
}

// NewSocReplicator replicator constructor
func NewSocReplicator(addr swarm.Address, rLevel redundancy.Level) *socReplicator {
	rr := &socReplicator{
		addr:   addr.Bytes(),
		sizes:  redundancy.GetReplicaCounts(),
		C:      make(chan *socReplica, 16),
		rLevel: rLevel,
	}
	go rr.replicas()
	return rr
}

func AllAddresses(addr swarm.Address) []swarm.Address {
	r := redundancy.PARANOID
	rr := NewSocReplicator(addr, r)
	addresses := make([]swarm.Address, r.GetReplicaCount())
	n := 0
	for r := range rr.C {
		addresses[n] = swarm.NewAddress(r.Addr)
		n++
	}
	return addresses
}

// socReplica of the mined SOC chunk (address) that serve as replicas
type socReplica struct {
	Addr  []byte // byte slice of the generated SOC address
	nonce uint8  // used nonce to generate the address
}

// replicate returns a replica for SOC seeded with a byte of entropy as argument
func (rr *socReplicator) replicate(i uint8) (sp *socReplica) {
	// calculate SOC address for potential replica
	h := swarm.NewHasher()
	_, _ = h.Write(rr.addr)
	_, _ = h.Write([]byte{i})
	return &socReplica{h.Sum(nil), i}
}

// replicas enumerates replica parameters (SOC ID) pushing it in a channel given as argument
// the order of replicas is so that addresses are always maximally dispersed
// in successive sets of addresses.
// I.e., the binary tree representing the new addresses prefix bits up to depth is balanced
func (rr *socReplicator) replicas() {
	defer close(rr.C)
	n := 0
	for i := uint8(0); n < rr.rLevel.GetReplicaCount() && i < 255; i++ {
		// create soc replica (ID and address using constant owner)
		// the soc is added to neighbourhoods of depths in the closed interval [from...to]
		r := rr.replicate(i)
		d, m := rr.add(r, rr.rLevel)
		if d == 0 {
			continue
		}
		for m, r = range rr.queue[n:] {
			if r == nil {
				break
			}
			rr.C <- r
		}
		n += m
	}
}

// add inserts the soc replica into a replicator so that addresses are balanced
func (rr *socReplicator) add(r *socReplica, rLevel redundancy.Level) (depth int, rank int) {
	if rLevel == redundancy.NONE {
		return 0, 0
	}
	nh := nh(rLevel, r.Addr)
	if rr.exist[nh] {
		return 0, 0
	}
	rr.exist[nh] = true
	l, o := rr.add(r, rLevel.Decrement())
	d := uint8(rLevel) - 1
	if l == 0 {
		o = rr.sizes[d]
		rr.sizes[d]++
		rr.queue[o] = r
		l = rLevel.GetReplicaCount()
	}
	return l, o
}

// UTILS

// index bases needed to keep track how many addresses were mined for a level.
var replicaIndexBases = [5]int{0, 2, 6, 14}

// nh returns the lookup key based on the redundancy level
// to be used as index to the replicators exist array
func nh(rLevel redundancy.Level, addr []byte) int {
	d := uint8(rLevel)
	return replicaIndexBases[d-1] + int(addr[0]>>(8-d))
}

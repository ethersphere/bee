// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package replicas implements a scheme to replicate SOC chunks
// in such a way that
// - the replicas are optimally dispersed to aid cross-neighbourhood redundancy
// - the replicas addresses can be deduced by retrievers only knowing the address
// of the original content addressed chunk
// - no new chunk validation rules are introduced
package replicas

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
	c      chan *socReplica
	rLevel redundancy.Level
}

// newSocReplicator replicator constructor
func newSocReplicator(addr swarm.Address, rLevel redundancy.Level) *socReplicator {
	rr := &socReplicator{
		addr:   addr.Bytes(),
		sizes:  redundancy.GetReplicaCounts(),
		c:      make(chan *socReplica, 16),
		rLevel: rLevel,
	}
	go rr.replicas()
	return rr
}

// socReplica of the mined SOC chunk (address) that serve as replicas
type socReplica struct {
	addr  []byte // byte slice of the generated SOC address
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
	defer close(rr.c)
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
			rr.c <- r
		}
		n += m
	}
}

// add inserts the soc replica into a replicator so that addresses are balanced
func (rr *socReplicator) add(r *socReplica, rLevel redundancy.Level) (depth int, rank int) {
	if rLevel == redundancy.NONE {
		return 0, 0
	}
	nh := nh(rLevel, r.addr)
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

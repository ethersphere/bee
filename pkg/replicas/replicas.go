// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package replicas implements a scheme to replicate chunks
// in such a way that
// - the replicas are optimally dispersed to aid cross-neighbourhood redundancy
// - the replicas addresses can be deduced by retrievers only knowing the address
// of the original content addressed chunk
// - no new chunk validation rules are introduced
package replicas

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	// RetryInterval is the duration between successive additional requests
	RetryInterval = 300 * time.Millisecond
	privKey, _    = crypto.DecodeSecp256k1PrivateKey(append([]byte{1}, make([]byte, 31)...))
	signer        = crypto.NewDefaultSigner(privKey)
)

// replicator running the find for replicas
type replicator struct {
	addr   []byte       // chunk address
	queue  [16]*replica // to sort addresses according to di
	exist  [30]bool     //  maps the 16 distinct nibbles on all levels
	sizes  [5]int       // number of distinct neighnourhoods redcorded for each depth
	c      chan *replica
	rLevel redundancy.Level
}

// newReplicator replicator constructor
func newReplicator(addr swarm.Address, rLevel redundancy.Level) *replicator {
	rr := &replicator{
		addr:   addr.Bytes(),
		sizes:  redundancy.GetReplicaCounts(),
		c:      make(chan *replica, 16),
		rLevel: rLevel,
	}
	go rr.replicas()
	return rr
}

// replica of the mined SOC chunk (address) that serve as replicas
type replica struct {
	addr, id []byte // byte slice of SOC address and SOC ID
}

// replicate returns a replica params structure seeded with a byte of entropy as argument
func (rr *replicator) replicate(i uint8) (sp *replica) {
	// change the last byte of the address to create SOC ID
	id := make([]byte, 32)
	copy(id, rr.addr)
	id[0] = i
	// calculate SOC address for potential replica
	h := swarm.NewHasher()
	_, _ = h.Write(id)
	_, _ = h.Write(swarm.ReplicasOwner)
	return &replica{h.Sum(nil), id}
}

// replicas enumerates replica parameters (SOC ID) pushing it in a channel given as argument
// the order of replicas is so that addresses are always maximally dispersed
// in successive sets of addresses.
// I.e., the binary tree representing the new addresses prefix bits up to depth is balanced
func (rr *replicator) replicas() {
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
func (rr *replicator) add(r *replica, rLevel redundancy.Level) (depth int, rank int) {
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

// UTILS

// index bases needed to keep track how many addresses were mined for a level.
var replicaIndexBases = [5]int{0, 2, 6, 14}

// nh returns the lookup key based on the redundancy level
// to be used as index to the replicators exist array
func nh(rLevel redundancy.Level, addr []byte) int {
	d := uint8(rLevel)
	return replicaIndexBases[d-1] + int(addr[0]>>(8-d))
}

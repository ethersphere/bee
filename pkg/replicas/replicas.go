// Copyright 2020 The Swarm Authors. All rights reserved.
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
	"context"
	"encoding/hex"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/file/redundancy"
	"github.com/ethersphere/bee/pkg/swarm"
)

type redundancyLevelType struct{}

var (
	// redundancyLevel is the context key for the redundancy level
	redundancyLevel redundancyLevelType
	// RetryInterval is the duration between successive additional requests
	RetryInterval = 300 * time.Millisecond
	//
	// counts of replicas used for levels of increasing security
	// the actual number of replicas needed to keep the error rate below 1/10^6
	// for the five levels of redundancy are 0, 2, 4, 5, 19
	// we use an approximation as the successive powers of 2
	counts     = [5]int{0, 2, 4, 8, 16}
	sums       = [5]int{0, 2, 6, 14, 30}
	privKey, _ = crypto.DecodeSecp256k1PrivateKey(append([]byte{1}, make([]byte, 31)...))
	signer     = crypto.NewDefaultSigner(privKey)
	owner, _   = hex.DecodeString("dc5b20847f43d67928f49cd4f85d696b5a7617b5")
)

// SetLevel sets the redundancy level in the context
func SetLevel(ctx context.Context, level redundancy.Level) context.Context {
	return context.WithValue(ctx, redundancyLevel, level)
}

// getLevelFromContext is a helper function to extract the redundancy level from the context
func getLevelFromContext(ctx context.Context) redundancy.Level {
	rlevel := redundancy.PARANOID
	if val := ctx.Value(redundancyLevel); val != nil {
		rlevel = val.(redundancy.Level)
	}
	return rlevel
}

// replicator running the find for replicas
type replicator struct {
	addr  []byte       // chunk address
	queue [16]*replica // to sort addresses according to di
	exist [30]bool     //  maps the 16 distinct nibbles on all levels
	sizes [5]int       // number of distinct neighnourhoods redcorded for each depth
	c     chan *replica
	depth uint8
}

// newReplicator replicator constructor
func newReplicator(addr swarm.Address, depth uint8) *replicator {
	rr := &replicator{
		addr:  addr.Bytes(),
		sizes: counts,
		c:     make(chan *replica, 16),
		depth: depth,
	}
	go rr.replicas()
	return rr
}

// replica of the mined SOC chunk (address) that serve as replicas
type replica struct {
	addr, id []byte // byte slice of SOC address and SOC ID
}

// replicate returns a replica params strucure seeded with a byte of entropy as argument
func (rr *replicator) replicate(i uint8) (sp *replica) {
	// change the last byte of the address to create SOC ID
	id := make([]byte, 32)
	copy(id, rr.addr)
	id[0] = i
	// calculate SOC address for potential replica
	h := swarm.NewHasher()
	_, _ = h.Write(id)
	_, _ = h.Write(owner)
	return &replica{h.Sum(nil), id}
}

// nh returns the lookup key for the neighbourhood of depth d
// to be used as index to the replicators exist array
func (r *replica) nh(d uint8) (nh int) {
	return sums[d-1] + int(r.addr[0]>>(8-d))
}

// replicas enumerates replica parameters (SOC ID) pushing it in a channel given as argument
// the order of replicas is so that addresses are always maximally dispersed
// in successive sets of addresses.
// I.e., the binary tree representing the new addresses prefix bits up to depth is balanced
func (rr *replicator) replicas() {
	defer close(rr.c)
	n := 0
	for i := uint8(0); n < counts[rr.depth] && i < 255; i++ {
		// create soc replica (ID and address using constant owner)
		// the soc is added to neighbourhoods of depths in the closed interval [from...to]
		r := rr.replicate(i)
		d, m := rr.add(r, rr.depth)
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
func (rr *replicator) add(r *replica, d uint8) (depth int, rank int) {
	if d == 0 {
		return 0, 0
	}
	nh := r.nh(d)
	if rr.exist[nh] {
		return 0, 0
	}
	rr.exist[nh] = true
	l, o := rr.add(r, d-1)
	if l == 0 {
		o = rr.sizes[d-1]
		rr.sizes[d-1]++
		rr.queue[o] = r
		l = int(d)
	}
	return l, o
}

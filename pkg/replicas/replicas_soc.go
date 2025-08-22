// Copyright 2025 The Swarm Authors. All rights reserved.
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
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// socReplicator running the find for replicas
type socReplicator struct {
	addr   []byte // chunk address
	sizes  [5]int // number of distinct neighbourhoods redcorded for each depth
	c      chan *socReplica
	rLevel redundancy.Level
}

// newSocReplicator socReplicator constructor
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
	addr  []byte // byte slice of SOC address
	nonce uint8  // byte of the mined nonce
}

// replicate returns a replica params structure seeded with a byte of entropy as argument
func (rr *socReplicator) replicate(i uint8, bitsRequired uint8) (sp *socReplica) {
	addr := make([]byte, 32)
	copy(addr, rr.addr)
	mirroredBits := mirrorBitsToMSB(i, bitsRequired)
	// zero out the first leading bitsRequired bits of addr[0] and set mirroredBits of `i`
	addr[0] &= 0xFF >> bitsRequired
	addr[0] |= mirroredBits
	if addr[0] == rr.addr[0] {
		// xor MSB after the mirrored bits because the iteration found the original address
		addr[0] ^= 1 << (bitsRequired - 1)
	}
	return &socReplica{addr: addr, nonce: addr[0]}
}

// replicas enumerates replica parameters (nonce) pushing it in a channel given as argument
// the order of replicas is so that addresses are always maximally dispersed
// in successive sets of addresses.
// I.e., the binary tree representing the new addresses prefix bits up to depth is balanced
func (rr *socReplicator) replicas() {
	defer close(rr.c)
	// number of bits required to represent all replicas
	bitsRequired := countBitsRequired(uint8(rr.rLevel.GetReplicaCount() - 1))
	// replicate iteration saturates all leading bits in generated addresses until bitsRequired
	for i := uint8(0); i < uint8(rr.rLevel.GetReplicaCount()); i++ {
		// create soc replica (with address and nonce)
		r := rr.replicate(i, bitsRequired)
		rr.c <- r
	}
}

// mirrorBitsToMSB mirrors the lowest n bits of v to the most significant bits of a byte.
// For example, mirrorBitsToMSB(0b00001101, 4) == 0b10110000
func mirrorBitsToMSB(v byte, n uint8) byte {
	var res byte
	for i := uint8(0); i < n; i++ {
		if (v & (1 << i)) != 0 {
			res |= (1 << (7 - i))
		}
	}
	return res
}

// countBitsRequired returns the minimum number of bits required to represent value v.
// For 0, it returns 1 (we need 1 bit to represent 0).
func countBitsRequired(v uint8) uint8 {
	if v == 0 {
		return 1
	}

	var bits uint8
	for v > 0 {
		bits++
		v >>= 1
	}
	return bits
}

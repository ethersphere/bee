// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"hash"
	//	"sync/atomic"
)

// BaseHasherFunc is a hash.Hash constructor function used for the base hash of the BMT.
// implemented by Keccak256 SHA3 sha3.NewLegacyKeccak256

// configuration
type TrConf struct {
	segmentSize  int      // size of leaf segments, stipulated to be = hash size
	segmentCount int      // the number of segments on the base level of the BMT
	capacity     int      // pool capacity, controls concurrency
	depth        int      // depth of the bmt trees = int(log2(segmentCount))+1
	maxSize      int      // the total length of the data (count * size)
	zerohashes   [][]byte // lookup table for predictable padding subtrees for all levels
	key          []byte
	hasher       BaseHasherFunc // base hasher to use for the BMT levels
}

// Pool provides a pool of trees used as resources by the BMT Hasher.
// A tree popped from the pool is guaranteed to have a clean state ready
// for hashing a new chunk.
type TrPool struct {
	c       chan *tree // the channel to obtain a resource from the pool
	*TrConf            // configuration
}

func NewTrConf(hasher BaseHasherFunc, key []byte, segmentCount, capacity int) *TrConf {
	count, depth := sizeToParams(segmentCount)
	segmentSize := hasher().Size()
	zerohashes := make([][]byte, depth+1)
	zeros := make([]byte, segmentSize)
	zerohashes[0] = zeros
	var err error
	// initialises the zerohashes lookup table
	for i := 1; i < depth+1; i++ {
		if zeros, err = doHash(hasher(), zeros, zeros); err != nil {
			panic(err.Error())
		}
		zerohashes[i] = zeros
	}
	return &TrConf{
		hasher:       hasher,
		key:          key,
		segmentSize:  segmentSize,
		segmentCount: segmentCount,
		capacity:     capacity,
		maxSize:      count * segmentSize,
		depth:        depth,
		zerohashes:   zerohashes,
	}
}

// NewPool creates a tree pool with hasher, segment size, segment count and capacity
// it reuses free trees or creates a new one if capacity is not reached.
func NewTrPool(c *TrConf) *TrPool {
	p := &TrPool{
		TrConf: c,
		c:      make(chan *tree, c.capacity),
	}
	for i := 0; i < c.capacity; i++ {
		p.c <- newTrTree(p.segmentSize, p.maxSize, p.depth, p.hasher)
	}
	return p
}

// Get returns a BMT hasher possibly reusing a tree from the pool
func (p *TrPool) Get() *TrHasher {
	t := <-p.c
	return &TrHasher{
		TrConf: p.TrConf,
		result: make(chan []byte),
		errc:   make(chan error, 1),
		span:   make([]byte, SpanSize),
		bmt:    t,
	}
}

// Put is called after using a bmt hasher to return the tree to a pool for reuse
func (p *TrPool) Put(h *TrHasher) {
	p.c <- h.bmt
}

// newTrNode constructs a segment hasher node in the BMT (used by newTrTree).
func newTrNode(index int, parent *node, hasher hash.Hash) *node {
	return &node{
		parent: parent,
		isLeft: index%2 == 0,
		hasher: hasher,
	}
}

// newTrTree initialises a tree by building up the nodes of a BMT
//
// segmentSize is stipulated to be the size of the hash.
func newTrTree(segmentSize, maxsize, depth int, hashfunc func() hash.Hash) *tree {
	n := newTrNode(0, nil, hashfunc())
	prevlevel := []*node{n}
	// iterate over levels and creates 2^(depth-level) nodes
	// the 0 level is on double segment sections so we start at depth - 2
	count := 2
	for level := depth - 2; level >= 0; level-- {
		nodes := make([]*node, count)
		for i := 0; i < count; i++ {
			parent := prevlevel[i/2]
			nodes[i] = newTrNode(i, parent, hashfunc())
		}
		prevlevel = nodes
		count *= 2
	}
	// the datanode level is the nodes on the last level
	return &tree{
		leaves: prevlevel,
		buffer: make([]byte, maxsize),
	}
}

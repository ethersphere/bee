// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"hash"
	"sync/atomic"
)

// BaseHasherFunc is a hash.Hash constructor function used for the base hash of the BMT.
// implemented by Keccak256 SHA3 sha3.NewLegacyKeccak256
type BaseHasherFunc func() hash.Hash

// configuration
type Conf struct {
	segmentSize  int            // size of leaf segments, stipulated to be = hash size
	segmentCount int            // the number of segments on the base level of the BMT
	capacity     int            // pool capacity, controls concurrency
	depth        int            // depth of the bmt trees = int(log2(segmentCount))+1
	maxSize      int            // the total length of the data (count * size)
	zerohashes   [][]byte       // lookup table for predictable padding subtrees for all levels
	hasher       BaseHasherFunc // base hasher to use for the BMT levels
}

// Pool provides a pool of trees used as resources by the BMT Hasher.
// A tree popped from the pool is guaranteed to have a clean state ready
// for hashing a new chunk.
type Pool struct {
	c     chan *tree // the channel to obtain a resource from the pool
	*Conf            // configuration
}

func NewConf(hasher BaseHasherFunc, segmentCount, capacity int) *Conf {
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
	return &Conf{
		hasher:       hasher,
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
func NewPool(c *Conf) *Pool {
	p := &Pool{
		Conf: c,
		c:    make(chan *tree, c.capacity),
	}
	for i := 0; i < c.capacity; i++ {
		p.c <- newTree(p.segmentSize, p.maxSize, p.depth, p.hasher)
	}
	return p
}

// Get returns a BMT hasher possibly reusing a tree from the pool
func (p *Pool) Get() *Hasher {
	t := <-p.c
	return &Hasher{
		Conf:   p.Conf,
		result: make(chan []byte),
		errc:   make(chan error, 1),
		span:   make([]byte, SpanSize),
		bmt:    t,
	}
}

// Put is called after using a bmt hasher to return the tree to a pool for reuse
func (p *Pool) Put(h *Hasher) {
	p.c <- h.bmt
}

// tree is a reusable control structure representing a BMT
// organised in a binary tree
//
// Hasher uses a Pool to obtain a tree for each chunk hash
// the tree is 'locked' while not in the pool.
type tree struct {
	leaves []*node // leaf nodes of the tree, other nodes accessible via parent links
	buffer []byte
}

// node is a reusable segment hasher representing a node in a BMT.
type node struct {
	isLeft      bool      // whether it is left side of the parent double segment
	parent      *node     // pointer to parent node in the BMT
	state       int32     // atomic increment impl concurrent boolean toggle
	left, right []byte    // this is where the two children sections are written
	hasher      hash.Hash // preconstructed hasher on nodes
}

// newNode constructs a segment hasher node in the BMT (used by newTree).
func newNode(index int, parent *node, hasher hash.Hash) *node {
	return &node{
		parent: parent,
		isLeft: index%2 == 0,
		hasher: hasher,
	}
}

// newTree initialises a tree by building up the nodes of a BMT
//
// segmentSize is stipulated to be the size of the hash.
func newTree(segmentSize, maxsize, depth int, hashfunc func() hash.Hash) *tree {
	n := newNode(0, nil, hashfunc())
	prevlevel := []*node{n}
	// iterate over levels and creates 2^(depth-level) nodes
	// the 0 level is on double segment sections so we start at depth - 2
	count := 2
	for level := depth - 2; level >= 0; level-- {
		nodes := make([]*node, count)
		for i := 0; i < count; i++ {
			parent := prevlevel[i/2]
			nodes[i] = newNode(i, parent, hashfunc())
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

// atomic bool toggle implementing a concurrent reusable 2-state object.
// Atomic addint with %2 implements atomic bool toggle.
// It returns true if the toggler just put it in the active/waiting state.
func (n *node) toggle() bool {
	return atomic.AddInt32(&n.state, 1)%2 == 1
}

// sizeToParams calculates the depth (number of levels) and segment count in the BMT tree.
func sizeToParams(n int) (c, d int) {
	c = 2
	for ; c < n; c *= 2 {
		d++
	}
	return c, d + 1
}

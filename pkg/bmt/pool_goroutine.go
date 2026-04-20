// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"hash"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// goroutineConf is the internal configuration for the goroutine BMT pool.
type goroutineConf struct {
	segmentSize  int
	segmentCount int
	capacity     int
	depth        int
	maxSize      int
	zerohashes   [][]byte
	prefix       []byte
	hasherFunc   func() hash.Hash
}

func (c *goroutineConf) baseHasher() hash.Hash {
	if len(c.prefix) > 0 {
		return swarm.NewPrefixHasher(c.prefix)
	}
	return swarm.NewHasher()
}

// goroutinePool is the goroutine-based BMT hasher pool.
type goroutinePool struct {
	c chan *goroutineTree
	*goroutineConf
}

func newGoroutineConf(prefix []byte, segmentCount, capacity int) *goroutineConf {
	count, depth := sizeToParams(segmentCount)
	segmentSize := SEGMENT_SIZE

	hasherFunc := func() hash.Hash {
		if len(prefix) > 0 {
			return swarm.NewPrefixHasher(prefix)
		}
		return swarm.NewHasher()
	}

	c := &goroutineConf{
		segmentSize:  segmentSize,
		segmentCount: segmentCount,
		capacity:     capacity,
		maxSize:      count * segmentSize,
		depth:        depth,
		prefix:       prefix,
		hasherFunc:   hasherFunc,
	}

	zerohashes := make([][]byte, depth+1)
	zeros := make([]byte, segmentSize)
	zerohashes[0] = zeros
	var err error
	for i := 1; i < depth+1; i++ {
		if zeros, err = doHash(c.baseHasher(), zeros, zeros); err != nil {
			panic(err.Error())
		}
		zerohashes[i] = zeros
	}
	c.zerohashes = zerohashes

	return c
}

// newGoroutinePool creates a goroutine BMT pool from a public Conf.
func newGoroutinePool(c *Conf) *goroutinePool {
	gc := newGoroutineConf(c.Prefix, c.SegmentCount, c.Capacity)
	p := &goroutinePool{
		goroutineConf: gc,
		c:             make(chan *goroutineTree, gc.capacity),
	}
	for i := 0; i < gc.capacity; i++ {
		p.c <- newGoroutineTree(gc.maxSize, gc.depth, gc.hasherFunc)
	}
	return p
}

// Get returns a BMT hasher, possibly reusing a tree from the pool.
func (p *goroutinePool) Get() Hasher {
	t := <-p.c
	return &goroutineHasher{
		goroutineConf: p.goroutineConf,
		result:        make(chan []byte),
		errc:          make(chan error, 1),
		span:          make([]byte, SpanSize),
		bmt:           t,
	}
}

// Put returns a hasher's tree to the pool for reuse.
func (p *goroutinePool) Put(h Hasher) {
	gh, ok := h.(*goroutineHasher)
	if !ok {
		panic("bmt: goroutinePool.Put called with non-goroutineHasher")
	}
	p.c <- gh.bmt
}

// goroutineTree is the tree structure used by the goroutine hasher.
type goroutineTree struct {
	leaves []*goroutineNode
	buffer []byte
}

// goroutineNode is a reusable segment hasher node in the goroutine BMT.
type goroutineNode struct {
	isLeft      bool
	parent      *goroutineNode
	state       int32
	left, right []byte
	hasher      hash.Hash
}

func newGoroutineNode(index int, parent *goroutineNode, hasher hash.Hash) *goroutineNode {
	return &goroutineNode{
		parent: parent,
		isLeft: index%2 == 0,
		hasher: hasher,
	}
}

func newGoroutineTree(maxsize, depth int, hashfunc func() hash.Hash) *goroutineTree {
	n := newGoroutineNode(0, nil, hashfunc())
	prevlevel := []*goroutineNode{n}
	count := 2
	for level := depth - 2; level >= 0; level-- {
		nodes := make([]*goroutineNode, count)
		for i := 0; i < count; i++ {
			parent := prevlevel[i/2]
			nodes[i] = newGoroutineNode(i, parent, hashfunc())
		}
		prevlevel = nodes
		count *= 2
	}
	return &goroutineTree{
		leaves: prevlevel,
		buffer: make([]byte, maxsize),
	}
}

// toggle implements atomic bool toggle for goroutineNode coordination.
func (n *goroutineNode) toggle() bool {
	return atomic.AddInt32(&n.state, 1)%2 == 1
}

// getSister returns a copy of the sibling section on the opposite side.
func (n *goroutineNode) getSister(isLeft bool) []byte {
	var src []byte
	if isLeft {
		src = n.right
	} else {
		src = n.left
	}
	buf := make([]byte, len(src))
	copy(buf, src)
	return buf
}

// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package bmt

import (
	"hash"

	"github.com/ethersphere/bee/v2/pkg/keccak"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// simdConf is the internal configuration for the SIMD BMT pool.
type simdConf struct {
	segmentSize  int
	segmentCount int
	capacity     int
	depth        int
	maxSize      int
	zerohashes   [][]byte
	prefix       []byte
	batchWidth   int
}

func (c *simdConf) baseHasher() hash.Hash {
	if len(c.prefix) > 0 {
		return swarm.NewPrefixHasher(c.prefix)
	}
	return swarm.NewHasher()
}

// simdPool is the SIMD-batched BMT hasher pool.
type simdPool struct {
	c chan *simdTree
	*simdConf
}

func newSIMDConf(prefix []byte, segmentCount, capacity int) *simdConf {
	count, depth := sizeToParams(segmentCount)
	segmentSize := SEGMENT_SIZE

	c := &simdConf{
		segmentSize:  segmentSize,
		segmentCount: segmentCount,
		capacity:     capacity,
		maxSize:      count * segmentSize,
		depth:        depth,
		prefix:       prefix,
		batchWidth:   keccak.BatchWidth(),
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

// newSIMDPool creates a SIMD BMT pool from a public Conf.
func newSIMDPool(c *Conf) *simdPool {
	sc := newSIMDConf(c.Prefix, c.SegmentCount, c.Capacity)
	p := &simdPool{
		simdConf: sc,
		c:        make(chan *simdTree, sc.capacity),
	}
	for i := 0; i < sc.capacity; i++ {
		p.c <- newSIMDTree(sc.maxSize, sc.depth, sc.baseHasher, sc.prefix)
	}
	return p
}

func (p *simdPool) Get() Hasher {
	t := <-p.c
	return &simdHasher{
		simdConf: p.simdConf,
		span:     make([]byte, SpanSize),
		bmt:      t,
	}
}

func (p *simdPool) Put(h Hasher) {
	sh, ok := h.(*simdHasher)
	if !ok {
		panic("bmt: simdPool.Put called with non-simdHasher")
	}
	p.c <- sh.bmt
}

// simdTree is the tree structure used by the SIMD hasher.
type simdTree struct {
	leaves     []*simdNode
	levels     [][]*simdNode
	buffer     []byte
	concat     [8][]byte
	leafConcat [8][]byte
}

// simdNode is a reusable segment hasher node in the SIMD BMT.
type simdNode struct {
	isLeft      bool
	parent      *simdNode
	left, right []byte
	hasher      hash.Hash
}

func newSIMDNode(index int, parent *simdNode, hasher hash.Hash) *simdNode {
	return &simdNode{
		parent: parent,
		isLeft: index%2 == 0,
		hasher: hasher,
		left:   make([]byte, hasher.Size()),
		right:  make([]byte, hasher.Size()),
	}
}

func newSIMDTree(maxsize, depth int, hashfunc func() hash.Hash, prefix []byte) *simdTree {
	prefixLen := len(prefix)
	n := newSIMDNode(0, nil, hashfunc())
	prevlevel := []*simdNode{n}
	allLevels := [][]*simdNode{prevlevel}
	count := 2
	for level := depth - 2; level >= 0; level-- {
		nodes := make([]*simdNode, count)
		for i := 0; i < count; i++ {
			parent := prevlevel[i/2]
			nodes[i] = newSIMDNode(i, parent, hashfunc())
		}
		allLevels = append(allLevels, nodes)
		prevlevel = nodes
		count *= 2
	}
	// reverse so levels[0]=leaves, levels[len-1]=root
	for i, j := 0, len(allLevels)-1; i < j; i, j = i+1, j-1 {
		allLevels[i], allLevels[j] = allLevels[j], allLevels[i]
	}
	segSize := hashfunc().Size()
	bufSize := prefixLen + 2*segSize
	var concat [8][]byte
	for i := range concat {
		concat[i] = make([]byte, bufSize)
		if prefixLen > 0 {
			copy(concat[i][:prefixLen], prefix)
		}
	}
	var leafConcat [8][]byte
	for i := range leafConcat {
		leafConcat[i] = make([]byte, prefixLen+2*segSize)
		if prefixLen > 0 {
			copy(leafConcat[i][:prefixLen], prefix)
		}
	}
	return &simdTree{
		leaves:     prevlevel,
		levels:     allLevels,
		buffer:     make([]byte, maxsize),
		concat:     concat,
		leafConcat: leafConcat,
	}
}

// getSister returns a copy of the sibling section on the opposite side.
func (n *simdNode) getSister(isLeft bool) []byte {
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

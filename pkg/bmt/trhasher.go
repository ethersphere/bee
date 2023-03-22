// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"hash"

	"github.com/ethersphere/bee/pkg/swarm"
)

func NewTrHasher(prefix []byte) *Hasher {
	capacity := 32
	hasherFact := func() hash.Hash { return swarm.NewTrHasher(prefix) }
	conf := NewConf(hasherFact, swarm.BmtBranches, capacity)

	return &Hasher{
		Conf:   conf,
		result: make(chan []byte),
		errc:   make(chan error, 1),
		span:   make([]byte, SpanSize),
		bmt:    newTree(conf.segmentSize, conf.maxSize, conf.depth, conf.hasher),
	}
}

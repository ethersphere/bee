// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package epochs implements time-based feeds using epochs as index
// and provide sequential as well as concurrent lookup algorithms
package epochs

import (
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
)

const (
	maxLevel = 32
)

var _ feeds.Index = (*epoch)(nil)

// epoch is referencing a slot in the epoch grid and represents an update
// it  implements the feeds.Index interface
type epoch struct {
	start uint64
	level uint8
}

func (e *epoch) String() string {
	return fmt.Sprintf("%d/%d", e.start, e.level)
}

// MarshalBinary implements the BinaryMarshaler interface
func (e *epoch) MarshalBinary() ([]byte, error) {
	epochBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(epochBytes, e.start)
	return crypto.LegacyKeccak256(append(epochBytes, e.level))
}

func next(e feeds.Index, last int64, at uint64) feeds.Index {
	if e == nil {
		return &epoch{0, maxLevel}
	}
	return e.Next(last, at)
}

// Next implements feeds.Index advancement
func (e *epoch) Next(last int64, at uint64) feeds.Index {
	if e.start+e.length() > at {
		return e.childAt(at)
	}
	return lca(int64(at), last).childAt(at)
}

// lca calculates the lowest common ancestor epoch given two unix times
func lca(at, after int64) *epoch {
	if after == 0 {
		return &epoch{0, maxLevel}
	}
	diff := uint64(at - after)
	length := uint64(1)
	var level uint8
	for level < maxLevel && (length < diff || uint64(at)/length != uint64(after)/length) {
		length <<= 1
		level++
	}
	start := (uint64(after) / length) * length
	return &epoch{start, level}
}

// parent returns the ancestor of an epoch
// the call is unsafe in that it must not be called on a toplevel epoch
func (e *epoch) parent() *epoch {
	length := e.length() << 1
	start := (e.start / length) * length
	return &epoch{start, e.level + 1}
}

// left returns the left sister of an epoch
// it is unsafe in that it must not be called on a left sister epoch
func (e *epoch) left() *epoch {
	return &epoch{e.start - e.length(), e.level}
}

// at returns the left of right child epoch of an epoch depending on where `at` falls
// it is unsafe in that it must not be called with an at that does not fall within the epoch
func (e *epoch) childAt(at uint64) *epoch {
	e = &epoch{e.start, e.level - 1}
	if at&e.length() > 0 {
		e.start |= e.length()
	}
	return e
}

// isLeft returns true if epoch is a left sister of its parent
func (e *epoch) isLeft() bool {
	return e.start&e.length() == 0
}

// length returns the span of the epoch
func (e *epoch) length() uint64 {
	return 1 << e.level
}

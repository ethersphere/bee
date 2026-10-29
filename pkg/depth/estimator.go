// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package depth estimates the network committed depth from the redistribution contract.
package depth

import (
	"sort"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	windowSize   = 10 // N
	windowRounds = 20 // samples older than this many rounds are dropped
	minSamples   = 5  // first value needs this many
)

type updateResult uint8

const (
	updateNone       updateResult = iota + 1 // no change
	updateFirst                              // first value set
	updateStep                               // moved one level toward the majority
	updateNoMajority                         // candidate had no strict majority
	updateTooFew                             // fewer than minSamples in the window, known or not
)

type sample struct {
	block    uint64
	txIndex  uint
	logIndex uint
	depth    uint8
}

func (s sample) compare(o sample) int {
	switch {
	case s.block != o.block:
		return cmp(s.block < o.block)
	case s.txIndex != o.txIndex:
		return cmp(s.txIndex < o.txIndex)
	case s.logIndex != o.logIndex:
		return cmp(s.logIndex < o.logIndex)
	}
	return 0
}

func cmp(less bool) int {
	if less {
		return -1
	}
	return 1
}

// estimator holds a window of recent depth samples ordered by
// (block, txIndex, logIndex) and the current depth estimate.
type estimator struct {
	samples  []sample
	estimate uint8
	known    bool
}

func newEstimator() *estimator {
	return &estimator{}
}

// add inserts s unless an equal (block, txIndex, logIndex) is present or s.depth > swarm.MaxPO.
// It keeps at most windowSize newest samples. It returns false when s was not added.
func (e *estimator) add(s sample) bool {
	if s.depth > swarm.MaxPO {
		return false
	}
	i := sort.Search(len(e.samples), func(i int) bool { return e.samples[i].compare(s) >= 0 })
	if i < len(e.samples) && e.samples[i].compare(s) == 0 {
		return false
	}
	if len(e.samples) >= windowSize && i == 0 {
		return false
	}
	e.samples = append(e.samples, sample{})
	copy(e.samples[i+1:], e.samples[i:])
	e.samples[i] = s
	if len(e.samples) > windowSize {
		e.samples = e.samples[len(e.samples)-windowSize:]
	}
	return true
}

// prune drops samples with block < oldest, where oldest = head - windowRounds*blocksPerRound clamped at 0.
func (e *estimator) prune(head, blocksPerRound uint64) {
	var oldest uint64
	if span := windowRounds * blocksPerRound; head > span {
		oldest = head - span
	}
	kept := e.samples[:0]
	for _, s := range e.samples {
		if s.block >= oldest {
			kept = append(kept, s)
		}
	}
	e.samples = kept
}

// update changes nothing and returns updateTooFew while len(samples) < minSamples, known or not.
// Otherwise it applies the first-value rule when !known, else the strict-majority
// rule with a one-level step.
func (e *estimator) update() updateResult {
	if len(e.samples) < minSamples {
		return updateTooFew
	}
	candidate, count := e.mode()
	if !e.known {
		e.estimate = candidate
		e.known = true
		return updateFirst
	}
	if count <= len(e.samples)/2 {
		return updateNoMajority
	}
	switch {
	case candidate > e.estimate:
		e.estimate++
		return updateStep
	case candidate < e.estimate:
		e.estimate--
		return updateStep
	}
	return updateNone
}

// mode returns the most frequent depth and its count; ties go to the lower depth.
func (e *estimator) mode() (uint8, int) {
	var counts [swarm.MaxPO + 1]int
	for _, s := range e.samples {
		counts[s.depth]++
	}
	var best uint8
	for d := range counts {
		if counts[d] > counts[best] {
			best = uint8(d)
		}
	}
	return best, counts[best]
}

func (e *estimator) depth() (uint8, bool) {
	return e.estimate, e.known
}

func (e *estimator) size() int {
	return len(e.samples)
}

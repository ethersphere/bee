// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth

type Sample struct {
	Block    uint64
	TxIndex  uint
	LogIndex uint
	Depth    uint8
}

type UpdateResult = updateResult

const (
	WindowSize   = windowSize
	WindowRounds = windowRounds
	MinSamples   = minSamples

	UpdateNone       = updateNone
	UpdateFirst      = updateFirst
	UpdateStep       = updateStep
	UpdateNoMajority = updateNoMajority
	UpdateTooFew     = updateTooFew
)

var NewEstimator = newEstimator

func (e *estimator) Add(s Sample) bool {
	return e.add(sample{block: s.Block, txIndex: s.TxIndex, logIndex: s.LogIndex, depth: s.Depth})
}
func (e *estimator) Prune(head, blocksPerRound uint64) { e.prune(head, blocksPerRound) }
func (e *estimator) Update() UpdateResult              { return e.update() }
func (e *estimator) Depth() (uint8, bool)              { return e.depth() }
func (e *estimator) Size() int                         { return e.size() }
func (e *estimator) Blocks() []uint64 {
	b := make([]uint64, 0, len(e.samples))
	for _, s := range e.samples {
		b = append(b, s.block)
	}
	return b
}

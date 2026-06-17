// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"math/big"
	"time"
)

const defaultInclusionMinProbability = 0.05

// inclusionProbability returns the chance of including a tx in the current block
// based on its effective tip relative to the network reference tip.
// Tips at or above the reference are always included.
func inclusionProbability(tip, refTip *big.Int, minProb float64) float64 {
	if refTip == nil || refTip.Sign() <= 0 {
		return 1.0
	}
	if tip.Cmp(refTip) >= 0 {
		return 1.0
	}

	tipF := new(big.Float).SetInt(tip)
	refF := new(big.Float).SetInt(refTip)
	ratioF := new(big.Float).Quo(tipF, refF)
	ratio, _ := ratioF.Float64()
	if ratio < 0 {
		ratio = 0
	}

	prob := ratio * ratio
	if prob < minProb {
		return minProb
	}
	if prob > 1 {
		return 1
	}
	return prob
}

func (s *SimChain) referenceInclusionTip() *big.Int {
	if s.backgroundTipMean != nil && s.backgroundTipMean.Sign() > 0 {
		return s.backgroundTipMean
	}
	return s.minMempoolTip
}

func (s *SimChain) shouldIncludeTx(entry *poolEntry, refTip *big.Int) bool {
	if !s.cfg.InclusionProbability {
		return true
	}

	tip := entry.effectiveTip(s.baseFee)
	prob := inclusionProbability(tip, refTip, s.cfg.InclusionMinProbability)
	if prob >= 1.0 {
		return true
	}
	return s.rng.Float64() < prob
}

func (s *SimChain) nextBlockPeriod() time.Duration {
	period := s.cfg.BlockPeriod
	if period <= 0 {
		period = 5 * time.Second
	}
	if s.cfg.BlockPeriodJitter <= 0 {
		return period
	}

	j := s.cfg.BlockPeriodJitter
	n := s.rng.Int63n(int64(j)*2 + 1)
	offset := time.Duration(n) - j
	total := period + offset
	if total < time.Millisecond {
		return time.Millisecond
	}
	return total
}

func (s *SimChain) blockTimeDelta() time.Duration {
	if s.scheduledBlockDelay > 0 {
		delay := s.scheduledBlockDelay
		s.scheduledBlockDelay = 0
		return delay
	}
	return s.nextBlockPeriod()
}

func (s *SimChain) recordInclusionDeferred() {
	s.stats.InclusionDeferred++
}

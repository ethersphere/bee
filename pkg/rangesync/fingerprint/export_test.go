// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Add16 and Sub16 export the packed four-lane arithmetic for tests.
var (
	Add16 = add16
	Sub16 = sub16
)

// CountNode and LtHashNode export the Group adapters of the Index slabs.
type (
	CountNode  = countNode
	LtHashNode = ltHashNode
)

// SetAfterConvertHook sets a function that Rebuild calls after it converted
// the new slab to Fenwick trees and before it swaps it in (rebuild step 5).
func SetAfterConvertHook(ix *Index, fn func()) {
	ix.afterConvert = fn
}

// IndexMemoryBytes returns the value of the index_memory_bytes gauge.
func IndexMemoryBytes(ix *Index) float64 {
	return metricValue(ix.metrics.MemoryBytes)
}

// IndexOutsideRoot returns the value of the index_outside_root_total counter.
func IndexOutsideRoot(ix *Index) float64 {
	return metricValue(ix.metrics.OutsideRoot)
}

func metricValue(m prometheus.Metric) float64 {
	var d dto.Metric
	if err := m.Write(&d); err != nil {
		panic(err)
	}
	if g := d.GetGauge(); g != nil {
		return g.GetValue()
	}
	return d.GetCounter().GetValue()
}

// SetChecksumBytes truncates Checksum to its first n bytes (the rest zero)
// until the test ends. Tests that use it must not run in parallel.
func SetChecksumBytes(t interface{ Cleanup(func()) }, n int) {
	old := checksumBytes
	checksumBytes = n
	t.Cleanup(func() { checksumBytes = old })
}

// SetPeelFactor sets the corruption cap on recoveries per received symbol
// until the test ends. Tests that use it must not run in parallel.
func SetPeelFactor(t interface{ Cleanup(func()) }, f uint64) {
	old := peelFactor
	peelFactor = f
	t.Cleanup(func() { peelFactor = old })
}

// NewFloatIteratorAt returns a FloatMapper iterator past index 0, at index i,
// with xoshiro256** state s.
func NewFloatIteratorAt(s [4]uint64, i uint64) IndexIterator {
	return &floatIterator{s: s, i: i, started: true}
}

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import "github.com/prometheus/client_golang/prometheus"

var (
	NewEvents           = newEvents
	SampleChunk         = sampleChunk
	MakeInclusionProofs = makeInclusionProofs
)

func (a *Agent) RoundSkippedCounter(reason string) prometheus.Counter {
	return a.metrics.RoundSkipped.WithLabelValues(reason)
}

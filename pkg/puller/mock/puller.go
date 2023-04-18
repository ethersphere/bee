// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

type mockRateReporter struct{ rate float64 }

func NewMockRateReporter(r float64) *mockRateReporter { return &mockRateReporter{r} }
func (m *mockRateReporter) SyncRate() float64         { return m.rate }

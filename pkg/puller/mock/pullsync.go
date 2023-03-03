// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"github.com/ethersphere/bee/pkg/pullsync"
)

type mockRateReporter struct{ rate float64 }

func NewMockRateReporter(r float64) pullsync.SyncRate { return &mockRateReporter{r} }
func (m *mockRateReporter) Rate() float64             { return m.rate }

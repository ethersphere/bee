// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"sync"
	"time"
)

type mockClock struct {
	mu   sync.Mutex
	time time.Time
}

func NewClock(t time.Time) *mockClock {
	return &mockClock{time: t}
}

// Now returns the current mock time.
func (mc *mockClock) Now() time.Time {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.time
}

// Advance advances the mock time by the given duration.
func (mc *mockClock) Advance(d time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.time = mc.time.Add(d)
}

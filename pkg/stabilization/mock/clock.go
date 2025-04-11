package mock

import (
	"sync"
	"time"
)

// mockClock allows controlling time in tests.
type mockClock struct {
	mu   sync.Mutex
	time time.Time
}

// NewMockClock creates a mock clock starting at time zero.
func NewMockClock() *mockClock {
	return &mockClock{time: time.Time{}} // Start at zero time
}

// Now returns the current mock time.
func (mc *mockClock) Now() time.Time {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.time
}

// Set sets the current mock time.
func (mc *mockClock) Set(t time.Time) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.time = t
}

// Advance advances the mock time by the given duration.
func (mc *mockClock) Advance(d time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.time = mc.time.Add(d)
}

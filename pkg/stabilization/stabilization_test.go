// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stabilization_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/stabilization"
	"github.com/ethersphere/bee/v2/pkg/stabilization/mock"
)

// Helper function to create a default config with a mock clock
func newTestConfig(clock stabilization.Clock) stabilization.Config {
	return stabilization.Config{
		PeriodDuration:             1 * time.Second,
		MinimumPeriods:             0,
		NumPeriodsForStabilization: 2,
		StabilizationFactor:        1.0,
		WarmupTime:                 30 * time.Second,
		Clock:                      clock,
	}
}

func TestNewDetector(t *testing.T) {
	clock := mock.NewClock(time.Now())

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := newTestConfig(clock)
		d, err := stabilization.NewDetector(cfg)
		if err != nil {
			t.Fatalf("Expected no error for valid config, got %v", err)
		}
		if d == nil {
			t.Fatal("Expected non-nil Detector")
		}
		if d.State() != stabilization.StateIdle {
			t.Errorf("Expected initial state to be Idle, got %s", d.State())
		}
		d.Close()
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		testCases := []struct {
			name        string
			modifier    func(*stabilization.Config)
			expectedErr string
		}{
			{
				name:        "ZeroPeriodDuration",
				modifier:    func(cfg *stabilization.Config) { cfg.PeriodDuration = 0 },
				expectedErr: "PeriodDuration must be positive",
			},
			{
				name:        "NegativePeriodDuration",
				modifier:    func(cfg *stabilization.Config) { cfg.PeriodDuration = -1 * time.Second },
				expectedErr: "PeriodDuration must be positive",
			},
			{
				name:        "NumPeriodsTooSmall",
				modifier:    func(cfg *stabilization.Config) { cfg.NumPeriodsForStabilization = 1 },
				expectedErr: "NumPeriodsForStabilization must be at least 2",
			},
			{
				name:        "NegativeStabilizationFactor",
				modifier:    func(cfg *stabilization.Config) { cfg.StabilizationFactor = -0.1 },
				expectedErr: "StabilizationFactor must be non-negative",
			},
			{
				name:        "NegativeMinimumPeriods",
				modifier:    func(cfg *stabilization.Config) { cfg.MinimumPeriods = -1 },
				expectedErr: "MinimumPeriods must be non-negative",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := newTestConfig(clock)
				tc.modifier(&cfg)
				_, err := stabilization.NewDetector(cfg)
				if err == nil {
					t.Fatalf("Expected error '%s', got nil", tc.expectedErr)
				}
				if err.Error() != tc.expectedErr {
					t.Errorf("Expected error '%s', got '%v'", tc.expectedErr, err)
				}
			})
		}
	})
}

func TestDetector_StateTransitions(t *testing.T) {
	startTime := time.Now().Truncate(time.Second)
	clock := mock.NewClock(startTime)
	cfg := newTestConfig(clock)
	cfg.StabilizationFactor = 0.1
	cfg.NumPeriodsForStabilization = 2
	cfg.MinimumPeriods = 0

	d, err := stabilization.NewDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}
	defer d.Close()

	var monitoringStarted bool
	var periodCompletedCount int
	var stabilized bool
	var stabilizedTotalCount int

	d.OnMonitoringStart = func(tm time.Time) {
		monitoringStarted = true
		if !tm.Equal(startTime) {
			t.Errorf("OnMonitoringStart time mismatch: expected %v, got %v", startTime, tm)
		}
	}
	d.OnPeriodComplete = func(tm time.Time, count int, stDev float64) {
		periodCompletedCount++
	}
	d.OnStabilized = func(tm time.Time, totalCount int) {
		stabilized = true
		stabilizedTotalCount = totalCount // Store the count
	}

	stabilizedCh, cancel := d.Subscribe()
	defer cancel()

	if d.State() != stabilization.StateIdle {
		t.Fatalf("Expected initial state Idle, got %s", d.State())
	}

	// 1. Idle -> Monitoring
	recordTime := d.Record() // Event 1 at t=0s. P1 count=1. Total=1.
	if !recordTime.Equal(startTime) {
		t.Fatalf("Record time mismatch: expected %v, got %v", startTime, recordTime)
	}
	if d.State() != stabilization.StateMonitoring {
		t.Fatalf("Expected state Monitoring after first event, got %s", d.State())
	}
	if !monitoringStarted {
		t.Error("OnMonitoringStart callback not called")
	}

	clock.Advance(cfg.PeriodDuration / 2) // t = 0.5s
	d.Record()                            // Event 2 at t=0.5s. P1 count=2. Total=2.

	clock.Advance(cfg.PeriodDuration) // t = 1.5s
	d.Record()                        // Event 3 at t=1.5s.
	// - Processes P1 completion [0s, 1s). Count was 2.
	// - OnPeriodComplete(t=1.0s, count=2, stDev=NaN). periodCompletedCount = 1.
	// - periodCounts = [2].
	// - checkStabilized() -> false (not enough periods).
	// - Starts P2 [1.0s, 2.0s). Records Event 3 in P2. P2 count=1. Total=3.

	if d.State() != stabilization.StateMonitoring {
		t.Fatalf("Expected Monitoring after P1, got %s", d.State())
	}
	if periodCompletedCount != 1 {
		t.Fatalf("Expected 1 period complete after P1 trigger, got %d", periodCompletedCount)
	}
	if stabilized {
		t.Fatal("Should not be stabilized after P1")
	}

	// --- Period 2 Events ---
	clock.Advance(cfg.PeriodDuration / 4) // t = 1.75s
	d.Record()                            // Event 4 at t=1.75s. P2 count=2. Total=4.

	// --- Trigger Period 2 Completion & Stabilization Check ---
	// Advance clock PAST the end of Period 2 (ends at 2.0s)
	clock.Advance(cfg.PeriodDuration) // t = 2.75s
	d.Record()                        // Event 5 at t=2.75s.
	// - Processes P2 completion [1.0s, 2.0s). Count was 2.
	// - OnPeriodComplete(t=2.0s, count=2, stDev=0.0). periodCompletedCount = 2.
	// - periodCounts = [2, 2].
	// - checkStabilized([2, 2]) -> StDev=0. 0 < 0.1 -> true.
	// - setStabilized() called. State -> Stabilized. OnStabilized called. Trigger called.
	// - Record returns immediately. Total=5.
	// - P3 starts at 2.0s, but Event 5 is *not* counted in it because Record returned early.

	if d.State() != stabilization.StateStabilized {
		t.Fatalf("Expected state Stabilized after P2, got %s", d.State())
	}
	if !stabilized {
		t.Error("OnStabilized callback not called")
	}
	expectedTotalCount := 5
	if stabilizedTotalCount != expectedTotalCount {
		t.Errorf("OnStabilized total count mismatch: expected %d, got %d", expectedTotalCount, stabilizedTotalCount)
	}
	if !d.IsReady() {
		t.Error("IsStabilized() should return true")
	}
	if periodCompletedCount != 2 {
		t.Errorf("Expected OnPeriodComplete to be called 2 times (for P1, P2), got %d", periodCompletedCount)
	}

	select {
	case <-stabilizedCh:
		// Stabilization notification received
	case <-time.After(20 * time.Millisecond): // Increased timeout slightly just in case
		t.Error("Subscription channel did not receive notification")
	}

	// 4. Stabilized -> Record ignored
	tBeforeRecord6 := clock.Now()
	recordTime = d.Record()
	if !recordTime.IsZero() {
		t.Errorf("Record() should return zero time when stabilized, got %v (clock time: %v)", recordTime, tBeforeRecord6)
	}
	if d.State() != stabilization.StateStabilized {
		t.Fatalf("Expected state Stabilized to persist after ignored record, got %s", d.State())
	}
	if periodCompletedCount != 2 {
		t.Errorf("OnPeriodComplete should not be called again after stabilization, count: %d", periodCompletedCount)
	}
}

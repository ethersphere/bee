// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stabilization provides a rate stabilization detector.
// It detects when a high-frequency burst of events ("peak")
// stabilizes into a period of significantly lower frequency.
// It uses a dynamic threshold based on the minimum interval observed during the peak.
package stabilization

import (
	"errors"
	"math"
	"sync"
	"time"

	"resenje.org/feed"
)

type Subscriber interface {
	// Subscribe returns a channel that will receive a notification when the
	// stabilization reaches the Stabilized state.
	Subscribe() (c <-chan struct{})
	// IsStabilized returns true if the detector is in the Stabilized state.
	IsStabilized() bool
}

// RateState represents the detected state of the event rate.
type RateState int

const (
	// StateIdle indicates no recent peak activity or initial state.
	StateIdle RateState = iota
	// StateInPeak indicates a high-frequency burst of events is occurring.
	StateInPeak
	// StateStabilized indicates the rate has slowed down significantly after a peak
	// and the detector will remain in this state.
	StateStabilized
)

const (
	subscriptionTopic int = 0
	maxDuration           = time.Duration(math.MaxInt64)
)

func (rs RateState) String() string {
	switch rs {
	case StateIdle:
		return "Idle"
	case StateInPeak:
		return "InPeak"
	case StateStabilized:
		return "Stabilized"
	default:
		return "Unknown"
	}
}

type Config struct {
	// RelativeSlowdownFactor: Multiplier for the peak's minimum interval duration
	// to determine the "slow" threshold. Must be > 1.0.
	RelativeSlowdownFactor float64
	// MinSlowSamples: The number of consecutive "slow" events required to detect
	// natural stabilization. Must be >= 1.
	MinSlowSamples int
	// WarmupTime: If stabilization is not detected naturally within this duration
	// after the peak starts, stabilization will be forced. Set to 0 to disable.
	WarmupTime time.Duration
	// Clock: Optional custom clock for testing. Defaults to SystemClock if nil.
	Clock Clock
}

// Detector detects when a high-frequency burst of events ("peak")
// stabilizes into a period of significantly lower frequency.
// It uses a dynamic threshold based on the minimum interval observed during the peak.
// Once stabilization is detected, it remains in the Stabilized state indefinitely.
type Detector struct {
	mutex sync.Mutex

	// Configuration
	relativeSlowdownFactor float64
	minSlowSamples         int
	warmupTime             time.Duration

	// State
	lastTimestamp        time.Time
	hasRecordedEvent     bool
	consecutiveSlowCount int
	currentState         RateState
	minDuration          time.Duration
	peakRateTimestamp    time.Time
	totalCount           int
	trigger              *feed.Trigger[int]
	warmupTimer          *time.Timer
	clock                Clock

	// Callbacks; do not call back into the detector it could cause deadlocks
	OnPeakStart    func(startTime time.Time)
	OnRateIncrease func(t time.Time, minDuration time.Duration)
	OnStabilized   func(t time.Time, totalCount int)
}

// NewDetector creates a new detector.
func NewDetector(cfg Config) (*Detector, error) {
	if cfg.RelativeSlowdownFactor <= 1.0 {
		return nil, errors.New("RelativeSlowdownFactor must be greater than 1.0")
	}

	if cfg.MinSlowSamples < 1 {
		return nil, errors.New("MinSlowSamples must be at least 1")
	}

	if cfg.WarmupTime < 0 {
		return nil, errors.New("WarmupTime cannot be negative")
	}

	clock := cfg.Clock
	if clock == nil {
		clock = SystemClock
	}

	return &Detector{
		relativeSlowdownFactor: cfg.RelativeSlowdownFactor,
		minSlowSamples:         cfg.MinSlowSamples,
		warmupTime:             cfg.WarmupTime,
		clock:                  clock,
		currentState:           StateIdle,
		minDuration:            maxDuration,
		trigger:                feed.NewTrigger[int](),
	}, nil
}

// Record records an event timestamp and updates the detection state.
// Returns the timestamp of the recorded event.
// If the state is already Stabilized, this function does nothing.
func (d *Detector) Record() time.Time {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	t := d.clock.Now()
	d.recordAt(t)
	return t
}

// recordAt records a specific event timestamp and updates the detection state.
// If the state is already Stabilized, this function does nothing.
func (d *Detector) recordAt(t time.Time) {
	if d.currentState == StateStabilized {
		return
	}

	d.totalCount++

	if !d.hasRecordedEvent {
		// First event recorded
		d.lastTimestamp = t
		d.hasRecordedEvent = true
		return
	}

	duration := t.Sub(d.lastTimestamp) // Calculate duration since the last event
	d.lastTimestamp = t                // Update last timestamp for the *next* calculation

	switch d.currentState {
	case StateIdle:
		d.currentState = StateInPeak
		d.minDuration = duration
		d.peakRateTimestamp = t
		d.consecutiveSlowCount = 0
		if d.OnPeakStart != nil {
			d.OnPeakStart(t)
		}
		if d.OnRateIncrease != nil && duration > 0 {
			d.OnRateIncrease(t, duration)
		}
		if d.warmupTime > 0 {
			d.startWarmupTimer(t)
		}

	case StateInPeak:
		if duration <= 0 {
			// Treat anomalies as "fast" events by resetting the slow count
			d.consecutiveSlowCount = 0
			return
		}

		// Update minimum duration if this event is faster
		if duration < d.minDuration {
			d.minDuration = duration
			d.peakRateTimestamp = t
			if d.OnRateIncrease != nil {
				d.OnRateIncrease(t, duration)
			}
		}

		// Check if the current interval is "slow" relative to the peak minimum
		isSlow := false
		if d.minDuration < maxDuration && d.minDuration > 0 {
			dynamicSlowThreshold := time.Duration(float64(d.minDuration) * d.relativeSlowdownFactor)
			// Handle potential overflow
			if dynamicSlowThreshold < d.minDuration {
				dynamicSlowThreshold = maxDuration
			}

			if duration >= dynamicSlowThreshold {
				isSlow = true
			}
		}

		if isSlow {
			d.consecutiveSlowCount++
			if d.consecutiveSlowCount >= d.minSlowSamples {
				d.transitionToStabilized(t)
			}
		} else {
			// Fast. Reset the slow count. Stay in InPeak state.
			d.consecutiveSlowCount = 0
		}
	}
}

// State returns the current detected rate state.
func (d *Detector) State() RateState {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.currentState
}

// Reset resets the detector to its initial state, allowing it to detect a new stabilization cycle.
func (d *Detector) Reset() {
	if d == nil {
		return
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.warmupTimer != nil {
		d.warmupTimer.Stop()
		d.warmupTimer = nil
	}

	d.currentState = StateIdle
	d.hasRecordedEvent = false
	d.consecutiveSlowCount = 0
	d.minDuration = maxDuration
	d.totalCount = 0
	d.lastTimestamp = time.Time{}
	d.peakRateTimestamp = time.Time{}
}

func (d *Detector) Subscribe() (c <-chan struct{}) {
	c, _ = d.trigger.Subscribe(subscriptionTopic)
	return c
}

func (d *Detector) IsStabilized() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.currentState == StateStabilized
}

func (d *Detector) startWarmupTimer(t time.Time) {
	if d.warmupTimer != nil {
		d.warmupTimer.Stop()
	}

	d.warmupTimer = time.AfterFunc(d.warmupTime, func() {
		d.mutex.Lock()
		defer d.mutex.Unlock()

		if d.currentState == StateInPeak {
			d.transitionToStabilized(t)
		}
	})
}

func (d *Detector) transitionToStabilized(t time.Time) {
	if d.warmupTimer != nil {
		d.warmupTimer.Stop()
		d.warmupTimer = nil
	}

	if d.currentState == StateInPeak {
		d.currentState = StateStabilized
		if d.OnStabilized != nil {
			d.OnStabilized(t, d.totalCount)
		}
		d.trigger.Trigger(subscriptionTopic)
	}
}

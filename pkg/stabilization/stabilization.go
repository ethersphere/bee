// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stabilization provides a rate stabilization detector.
// It detects when the rate of events becomes stable over a
// configured number of time periods.
package stabilization

import (
	"errors"
	"sync"
	"time"

	"resenje.org/feed"
)

var _ Subscriber = (*Detector)(nil)

// Subscriber defines the interface for stabilization subscription.
type Subscriber interface {
	// Subscribe returns a channel that will receive a notification when the
	// stabilization reaches the Stabilized state.
	Subscribe() (c <-chan struct{})
	// IsStabilized returns true if the detector is in the Stabilized state.
	IsStabilized() bool
}

// RateState represents the detected state of the event rate stabilization.
type RateState int

const (
	// StateIdle indicates the detector is inactive, waiting for the first event.
	StateIdle RateState = iota
	// StateMonitoring indicates events are being recorded and checked for rate stability over periods.
	StateMonitoring
	// StateStabilized indicates the event rate has been consistent for the configured
	// number of periods, or the warmup time has elapsed. The detector will remain in this state.
	StateStabilized
)

const (
	subscriptionTopic int = 0
)

func (rs RateState) String() string {
	switch rs {
	case StateIdle:
		return "Idle"
	case StateMonitoring:
		return "Monitoring"
	case StateStabilized:
		return "Stabilized"
	default:
		return "Unknown"
	}
}

// Config holds the configuration parameters for the rate stabilization detector.
type Config struct {
	// PeriodDuration is the length of each time period for calculating event rates.
	// Must be greater than zero.
	PeriodDuration time.Duration
	// NumPeriodsForStabilization is the number of consecutive recent periods to check
	// for rate stabilization. Must be at least 2.
	NumPeriodsForStabilization int
	// StabilizationFactor is the maximum allowed relative difference between the highest
	// and lowest event counts in the stabilization window.
	StabilizationFactor float64
	// WarmupTime forces stabilization if not detected naturally within this duration
	// after monitoring starts. Set to 0 or negative to disable.
	WarmupTime time.Duration
	// Clock is an optional custom clock for testing. Defaults to SystemClock if nil.
	Clock Clock
}

// Detector detects when the rate of events stabilizes over a defined period.
// It calculates the number of events per period and checks if the rates
// in the last 'NumPeriodsForStabilization' are similar within a given factor.
type Detector struct {
	mutex sync.Mutex

	// Configuration
	periodDuration             time.Duration
	numPeriodsForStabilization int
	stabilizationFactor        float64
	warmupTime                 time.Duration
	clock                      Clock

	// State
	currentState        RateState
	periodCounts        []int       // Stores counts for the last NumPeriodsForStabilization periods
	currentPeriodCount  int         // Event count for the *current* active period
	totalCount          int         // Total events recorded since the start or last reset
	monitoringStartTime time.Time   // Time when monitoring state began
	periodTimer         *time.Timer // Timer triggering end of periods
	warmupTimer         *time.Timer // Timer for forced stabilization fallback
	trigger             *feed.Trigger[int]

	// Callbacks - DO NOT call back into the detector from these; it can cause deadlocks.
	OnMonitoringStart func(t time.Time)
	OnPeriodComplete  func(t time.Time, rate int) // Rate is events in the completed period
	OnStabilized      func(t time.Time, totalCount int)
}

// NewDetector creates a new rate stabilization detector.
func NewDetector(cfg Config) (*Detector, error) {
	if cfg.PeriodDuration <= 0 {
		return nil, errors.New("PeriodDuration must be positive")
	}

	if cfg.NumPeriodsForStabilization < 2 {
		return nil, errors.New("NumPeriodsForStabilization must be at least 2")
	}

	if cfg.StabilizationFactor < 0.0 {
		return nil, errors.New("StabilizationFactor must be non-negative")
	}

	clock := cfg.Clock
	if clock == nil {
		clock = SystemClock
	}

	return &Detector{
		periodDuration:             cfg.PeriodDuration,
		numPeriodsForStabilization: cfg.NumPeriodsForStabilization,
		stabilizationFactor:        cfg.StabilizationFactor,
		warmupTime:                 cfg.WarmupTime,
		clock:                      clock,
		currentState:               StateIdle,
		periodCounts:               make([]int, 0, cfg.NumPeriodsForStabilization),
		trigger:                    feed.NewTrigger[int](),
	}, nil
}

// Record signals that an event has occurred. It updates the internal state
// and may trigger state transitions or callbacks.
// Returns the timestamp when the event was recorded.
// If the state is already Stabilized, this function does nothing and returns zero time.
func (d *Detector) Record() time.Time {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.currentState == StateStabilized {
		return time.Time{}
	}

	t := d.clock.Now()

	if d.currentState == StateIdle {
		d.currentState = StateMonitoring
		d.monitoringStartTime = t
		if d.OnMonitoringStart != nil {
			d.OnMonitoringStart(t)
		}
		d.startPeriodTimer()
		d.startWarmupTimer(t)
	}

	d.totalCount++
	d.currentPeriodCount++

	return t
}

// State returns the current detected rate state.
func (d *Detector) State() RateState {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.currentState
}

// Reset resets the detector to its initial StateIdle, clearing all counts and timers.
// This allows the detector to monitor for stabilization again.
func (d *Detector) Reset() {
	if d == nil {
		return
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.periodTimer != nil {
		d.periodTimer.Stop()
		d.periodTimer = nil
	}
	if d.warmupTimer != nil {
		d.warmupTimer.Stop()
		d.warmupTimer = nil
	}
	d.currentState = StateIdle
	d.periodCounts = make([]int, 0, d.numPeriodsForStabilization)
	d.currentPeriodCount = 0
	d.totalCount = 0
	d.monitoringStartTime = time.Time{}
}

// Subscribe returns a channel that receives an empty struct notification
// when the detector transitions to the StateStabilized.
// The channel is buffered and closed after the notification.
func (d *Detector) Subscribe() (c <-chan struct{}) {
	c, _ = d.trigger.Subscribe(subscriptionTopic) // Ignores the unsubscribe function
	return c
}

// IsStabilized returns true if the detector is currently in the StateStabilized.
func (d *Detector) IsStabilized() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.currentState == StateStabilized
}

func (d *Detector) startPeriodTimer() {
	d.periodTimer = time.AfterFunc(d.periodDuration, func() {
		d.mutex.Lock()
		if d.currentState != StateMonitoring {
			d.mutex.Unlock()
			return
		}

		// Process the period
		completedCount := d.currentPeriodCount
		d.periodCounts = append(d.periodCounts, completedCount)
		if len(d.periodCounts) > d.numPeriodsForStabilization {
			d.periodCounts = d.periodCounts[1:]
		}
		d.currentPeriodCount = 0

		if d.OnPeriodComplete != nil {
			d.OnPeriodComplete(d.clock.Now(), completedCount)
		}

		// Check for stabilization
		if len(d.periodCounts) == d.numPeriodsForStabilization {
			min := d.periodCounts[0]
			max := d.periodCounts[0]
			for _, count := range d.periodCounts[1:] {
				if count < min {
					min = count
				}
				if count > max {
					max = count
				}
			}
			if min == max || (min > 0 && float64(max-min)/float64(min) <= d.stabilizationFactor) {
				d.currentState = StateStabilized
				if d.periodTimer != nil {
					d.periodTimer.Stop()
				}
				if d.warmupTimer != nil {
					d.warmupTimer.Stop()
				}
				if d.OnStabilized != nil {
					d.OnStabilized(d.clock.Now(), d.totalCount)
				}
				d.trigger.Trigger(subscriptionTopic)
				d.mutex.Unlock()
				return
			}
		}

		// Schedule the next period
		d.startPeriodTimer()
		d.mutex.Unlock()
	})
}

func (d *Detector) startWarmupTimer(t time.Time) {
	if d.warmupTimer != nil {
		d.warmupTimer.Stop()
	}

	d.warmupTimer = time.AfterFunc(d.warmupTime, func() {
		d.mutex.Lock()
		defer d.mutex.Unlock()

		if d.currentState == StateMonitoring {
			d.currentState = StateStabilized
			if d.periodTimer != nil {
				d.periodTimer.Stop()
			}
			if d.OnStabilized != nil {
				d.OnStabilized(t, d.totalCount)
			}
			d.trigger.Trigger(subscriptionTopic)
		}
	})
}

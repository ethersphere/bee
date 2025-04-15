// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stabilization provides a rate stabilization detector.
// It detects when the rate of events becomes stable over a
// configured number of time periods.
package stabilization

import (
	"errors"
	"math"
	"sync"
	"time"

	"resenje.org/feed"
)

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
	// Must be greater than zero. Time between measurements.
	PeriodDuration time.Duration
	// MinimumPeriods is the number of initial periods to wait *before*
	// the NumPeriodsForStabilization window is considered for stabilization checks.
	MinimumPeriods int
	// NumPeriodsForStabilization is the number of consecutive recent periods to check
	// for rate stabilization. Must be at least 2.
	NumPeriodsForStabilization int
	// Stability threshold: Maximum acceptable deviation (lower = more strict).
	StabilizationFactor float64
	// WarmupTime forces stabilization if not detected naturally within this duration
	// after monitoring starts.
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
	minimumPeriods             int
	warmupTime                 time.Duration
	clock                      Clock

	// State
	currentState           RateState
	totalCount             int
	currentPeriodCount     int
	currentPeriodStartTime time.Time
	periodCounts           []int
	trigger                *feed.Trigger[int]
	warmupTimer            *time.Timer

	OnMonitoringStart func(t time.Time)
	OnPeriodComplete  func(t time.Time, countInPeriod int, stDev float64)
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

	if cfg.MinimumPeriods < 0 {
		return nil, errors.New("MinimumPeriods must be non-negative")
	}

	clock := cfg.Clock
	if clock == nil {
		clock = SystemClock
	}

	//
	minimumPeriods := cfg.MinimumPeriods + cfg.NumPeriodsForStabilization

	return &Detector{
		periodDuration:             cfg.PeriodDuration,
		numPeriodsForStabilization: cfg.NumPeriodsForStabilization,
		stabilizationFactor:        cfg.StabilizationFactor,
		minimumPeriods:             minimumPeriods,
		warmupTime:                 cfg.WarmupTime,
		clock:                      clock,
		currentState:               StateIdle,
		trigger:                    feed.NewTrigger[int](),
		periodCounts:               make([]int, 0, minimumPeriods),
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

	d.totalCount++
	t := d.clock.Now()

	switch d.currentState {
	case StateIdle:
		d.currentState = StateMonitoring
		d.currentPeriodStartTime = t
		d.currentPeriodCount = 1
		if d.OnMonitoringStart != nil {
			d.OnMonitoringStart(t)
		}
		d.startWarmupTimer(t)

	case StateMonitoring:
		for t.Sub(d.currentPeriodStartTime) >= d.periodDuration {
			completedPeriodEndTime := d.currentPeriodStartTime.Add(d.periodDuration)

			d.periodCounts = append(d.periodCounts, d.currentPeriodCount)
			if len(d.periodCounts) > d.minimumPeriods {
				// remove old periods
				d.periodCounts = d.periodCounts[len(d.periodCounts)-d.minimumPeriods:]
			}

			isStable, stDev := d.checkStabilized()

			if d.OnPeriodComplete != nil {
				d.OnPeriodComplete(completedPeriodEndTime, d.currentPeriodCount, stDev)
			}

			d.currentPeriodCount = 0                          // reset the count for the next period
			d.currentPeriodStartTime = completedPeriodEndTime // start of the next period

			if isStable {
				d.setStabilized(t)
				return t
			}
		}
		d.currentPeriodCount++
	}

	return t
}

// State returns the current detected rate state.
func (d *Detector) State() RateState {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.currentState
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

// Close stops the detector and releases any resources.
func (d *Detector) Close() {
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
	d.totalCount = 0
	d.currentPeriodCount = 0
	d.currentPeriodStartTime = time.Time{}
	d.periodCounts = d.periodCounts[:0]
}

func (d *Detector) startWarmupTimer(t time.Time) {
	if d.warmupTimer != nil {
		d.warmupTimer.Stop()
	}

	d.warmupTimer = time.AfterFunc(d.warmupTime, func() {
		d.mutex.Lock()
		defer d.mutex.Unlock()

		if d.currentState == StateMonitoring {
			d.setStabilized(t)
		}
	})
}

func (d *Detector) setStabilized(t time.Time) {
	if d.warmupTimer != nil {
		d.warmupTimer.Stop()
		d.warmupTimer = nil
	}

	if d.currentState == StateMonitoring {
		d.currentState = StateStabilized
		if d.OnStabilized != nil {
			d.OnStabilized(t, d.totalCount)
		}
		d.trigger.Trigger(subscriptionTopic)
	}
}

func (d *Detector) checkStabilized() (bool, float64) {
	if len(d.periodCounts) < d.minimumPeriods {
		return false, math.NaN()
	}

	startIndex := len(d.periodCounts) - d.numPeriodsForStabilization
	relevantCounts := d.periodCounts[startIndex:]
	stDev := calculateStDev(relevantCounts)

	isStabilized := !math.IsNaN(stDev) && stDev < d.stabilizationFactor

	return isStabilized, stDev
}

func calculateStDev(relevantCounts []int) float64 {
	n := len(relevantCounts)
	if n < 2 {
		return math.NaN()
	}

	var sum float64
	for _, count := range relevantCounts {
		sum += float64(count)
	}
	mean := sum / float64(n)

	sumSquaredDiff := 0.0
	for _, count := range relevantCounts {
		diff := float64(count) - mean
		sumSquaredDiff += diff * diff
	}

	// sample variance
	variance := sumSquaredDiff / float64(n-1)

	return math.Sqrt(variance)
}

package stabilization

import (
	"errors"
	"math"
	"sync"
	"time"
)

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

// Detector detects when a high-frequency burst of events ("peak")
// stabilizes into a period of significantly lower frequency.
// It uses a dynamic threshold based on the minimum interval observed during the peak.
// Once stabilization is detected, it remains in the Stabilized state indefinitely.
type Detector struct {
	mutex sync.Mutex

	// Configuration
	relativeSlowdownFactor float64
	minSlowSamples         int

	// State
	lastTimestamp        time.Time
	hasRecordedEvent     bool
	consecutiveSlowCount int
	currentState         RateState
	minDuration          time.Duration
	peakRateTimestamp    time.Time
	totalCount           int

	// Callbacks; do not call back into the detector it could cause deadlocks
	OnPeakStart    func(startTime time.Time)
	OnRateIncrease func(t time.Time, minDuration time.Duration)
	OnStabilized   func(t time.Time, totalCount int)
}

// NewDetector creates a new detector.
//   - relativeSlowdownFactor: Multiplier for the peak's minimum interval duration
//     to determine the "slow" threshold. Must be > 1.0.
//   - minSlowSamples: The number of consecutive "slow" events required to detect stabilization. Must be >= 1.
func NewDetector(relativeSlowdownFactor float64, minSlowSamples int) (*Detector, error) {
	if relativeSlowdownFactor <= 1.0 {
		return nil, errors.New("relativeSlowdownFactor must be greater than 1.0")
	}
	if minSlowSamples < 1 {
		return nil, errors.New("minSlowSamples must be at least 1")
	}

	return &Detector{
		relativeSlowdownFactor: relativeSlowdownFactor,
		minSlowSamples:         minSlowSamples,
		currentState:           StateIdle,
		minDuration:            time.Duration(math.MaxInt64), // max duration to start
	}, nil
}

// Record records an event timestamp and updates the detection state.
// Returns the timestamp of the recorded event.
// If the state is already Stabilized, this function does nothing.
func (d *Detector) Record() time.Time {
	if d == nil {
		return time.Time{}
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	t := time.Now()
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
	maxDuration := time.Duration(math.MaxInt64)

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
				d.currentState = StateStabilized
				if d.OnStabilized != nil {
					d.OnStabilized(t, d.totalCount)
				}
			}
		} else {
			// Fast. Reset the slow count. Stay in InPeak state.
			d.consecutiveSlowCount = 0
		}
	}
}

// State returns the current detected rate state.
func (d *Detector) State() RateState {
	if d == nil {
		return StateIdle
	}

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
	d.currentState = StateIdle
	d.hasRecordedEvent = false
	d.consecutiveSlowCount = 0
	d.minDuration = time.Duration(math.MaxInt64)
	d.totalCount = 0
}

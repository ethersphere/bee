package stabilization

import "time"

// Clock interface for abstracting time operations
type Clock interface {
	Now() time.Time
}

// systemClock implements Clock using the standard time package
type systemClock struct{}

func (sc *systemClock) Now() time.Time {
	return time.Now()
}

func (c systemClock) AfterFunc(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}

// Use SystemClock as the default
var SystemClock Clock = &systemClock{}

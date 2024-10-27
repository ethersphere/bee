// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breaker_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/breaker"
)

func TestExecute(t *testing.T) {
	t.Parallel()

	testErr := errors.New("test error")
	shouldNotBeCalledErr := errors.New("should not be called")
	failInterval := 10 * time.Minute
	startBackoff := 1 * time.Minute
	initTime := time.Now()

	testCases := map[string]struct {
		limit        int
		ferrors      []error
		iterations   int
		times        []time.Time
		expectedErrs []error
	}{
		"f() returns nil": {
			limit:        5,
			iterations:   1,
			ferrors:      []error{nil},
			times:        nil,
			expectedErrs: []error{nil},
		},
		"f() returns error": {
			limit:        5,
			ferrors:      []error{testErr},
			iterations:   1,
			times:        nil,
			expectedErrs: []error{testErr},
		},
		"Break error": {
			limit:        1,
			ferrors:      []error{testErr, shouldNotBeCalledErr},
			iterations:   3,
			times:        nil,
			expectedErrs: []error{testErr, breaker.ErrClosed, breaker.ErrClosed},
		},
		"Break error - mix iterations": {
			limit:        3,
			ferrors:      []error{testErr, nil, testErr, testErr, testErr, shouldNotBeCalledErr},
			iterations:   6,
			times:        nil,
			expectedErrs: []error{testErr, nil, testErr, testErr, testErr, breaker.ErrClosed},
		},
		"Expiration - return f() error": {
			limit:        3,
			ferrors:      []error{testErr, testErr, testErr, testErr, testErr},
			iterations:   5,
			times:        []time.Time{initTime, initTime, initTime.Add(2 * failInterval), initTime, initTime, initTime, initTime},
			expectedErrs: []error{testErr, testErr, testErr, testErr, testErr},
		},
		"Backoff - close, reopen, close, don't open": {
			limit:        1,
			ferrors:      []error{testErr, shouldNotBeCalledErr, testErr, shouldNotBeCalledErr, testErr, shouldNotBeCalledErr, shouldNotBeCalledErr},
			iterations:   7,
			times:        []time.Time{initTime, initTime, initTime, initTime.Add(startBackoff + time.Second), initTime, initTime, initTime, initTime.Add(2*startBackoff + time.Second), initTime, initTime, initTime, initTime.Add(startBackoff + time.Second)},
			expectedErrs: []error{testErr, breaker.ErrClosed, testErr, breaker.ErrClosed, testErr, breaker.ErrClosed, breaker.ErrClosed},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctMock := &currentTimeMock{
				times: tc.times,
			}

			b := breaker.NewBreakerWithCurrentTimeFn(breaker.Options{
				Limit:        tc.limit,
				StartBackoff: startBackoff,
				FailInterval: failInterval,
			}, ctMock.Time)

			for i := 0; i < tc.iterations; i++ {
				if err := b.Execute(func() error {
					if errors.Is(tc.ferrors[i], shouldNotBeCalledErr) {
						t.Fatal(tc.ferrors[i])
					}

					return tc.ferrors[i]
				}); !errors.Is(err, tc.expectedErrs[i]) {
					t.Fatalf("expected err: %s, got: %s, iteration %v", tc.expectedErrs[i], err, i)
				}
			}
		})
	}
}

func TestClosedUntil(t *testing.T) {
	t.Parallel()

	timestamp := time.Now()
	startBackoff := 1 * time.Minute
	testError := errors.New("test error")
	ctMock := &currentTimeMock{
		times: []time.Time{timestamp, timestamp, timestamp},
	}

	b := breaker.NewBreakerWithCurrentTimeFn(breaker.Options{
		Limit:        1,
		StartBackoff: startBackoff,
	}, ctMock.Time)

	notClosed := b.ClosedUntil()
	if notClosed != timestamp {
		t.Fatalf("expected: %s, got: %s", timestamp, notClosed)
	}

	if err := b.Execute(func() error {
		return testError
	}); !errors.Is(err, testError) {
		t.Fatalf("expected nil got %s", err)
	}

	closed := b.ClosedUntil()
	if closed != timestamp.Add(startBackoff) {
		t.Fatalf("expected: %s, got: %s", timestamp.Add(startBackoff), notClosed)
	}
}

type currentTimeMock struct {
	times []time.Time
	curr  int
}

func (c *currentTimeMock) Time() time.Time {
	if c.times == nil {
		return time.Now()
	}

	t := c.times[c.curr]
	c.curr++
	return t
}

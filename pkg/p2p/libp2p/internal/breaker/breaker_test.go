// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breaker_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/breaker"
)

var (
	testErr              = errors.New("test error")
	shouldNotBeCalledErr = errors.New("should not be called")
)

func TestExecute(t *testing.T) {
	testCases := map[string]struct {
		limit        int
		ferrors      []error
		iterations   int
		timeOffsets  []time.Duration
		expectedErrs []error
	}{
		"f() returns nil": {
			limit:        5,
			iterations:   1,
			ferrors:      []error{nil},
			timeOffsets:  nil,
			expectedErrs: []error{nil},
		},
		"f() returns error": {
			limit:        5,
			ferrors:      []error{testErr},
			iterations:   1,
			timeOffsets:  nil,
			expectedErrs: []error{testErr},
		},
		"Break error - zero limit": {
			limit:        0,
			ferrors:      []error{shouldNotBeCalledErr},
			iterations:   3,
			timeOffsets:  nil,
			expectedErrs: []error{breaker.ErrClosed, breaker.ErrClosed, breaker.ErrClosed},
		},
		"Break error - multiple iterations": {
			limit:        3,
			ferrors:      []error{testErr, testErr, testErr, shouldNotBeCalledErr, shouldNotBeCalledErr},
			iterations:   5,
			timeOffsets:  nil,
			expectedErrs: []error{testErr, testErr, testErr, breaker.ErrClosed, breaker.ErrClosed},
		},
		"Expiration - return f() error": {
			limit:        3,
			ferrors:      []error{testErr, testErr, testErr, testErr, testErr},
			iterations:   5,
			timeOffsets:  []time.Duration{time.Second, time.Second, 2 * breaker.GetFailInterval(), time.Second, time.Second},
			expectedErrs: []error{testErr, testErr, testErr, testErr, testErr},
		},
		"Backoff - close, reopen": {
			limit:        3,
			ferrors:      []error{testErr, testErr, testErr, shouldNotBeCalledErr, shouldNotBeCalledErr, testErr, testErr},
			iterations:   7,
			timeOffsets:  []time.Duration{time.Second, time.Second, time.Second, time.Second, time.Second, 2 * breaker.GetBackoffLimit(), time.Second},
			expectedErrs: []error{testErr, testErr, testErr, breaker.ErrClosed, breaker.ErrClosed, testErr, testErr},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			b := breaker.NewBreaker(tc.limit)
			for i := 0; i < tc.iterations; i++ {
				if tc.timeOffsets != nil {
					breaker.SetTimeNow(func() time.Time {
						return time.Now().Add(tc.timeOffsets[i])
					})
				} else {
					breaker.SetTimeNow(time.Now)
				}

				if err := b.Execute(func() error { return tc.ferrors[i] }); err != tc.expectedErrs[i] {
					t.Fatalf("expected err: %s, got: %s, iteration %v", tc.expectedErrs[i], err, i)
				}
			}
		})
	}
}

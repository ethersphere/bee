// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest_test

import (
	"errors"
	"fmt"
	"testing"
)

// assert is a test helper that validates a functionality of another helper
// function by mocking Errorf, Fatal and Helper methods on testing.TB.
func assert(t *testing.T, wantError, wantFatal string, f func(m *mock)) {
	t.Helper()

	defer func() {
		if v := recover(); v != nil {
			if err, ok := v.(error); ok && errors.Is(err, errFailed) {
				return // execution of the goroutine is stopped by a mock Fatal function
			}
			t.Fatalf("panic: %v", v)
		}
	}()

	m := &mock{
		wantError: wantError,
		wantFatal: wantFatal,
	}

	f(m)

	if !m.isHelper { // Request function is tested and it must be always a helper
		t.Error("not a helper function")
	}

	if m.gotError != m.wantError {
		t.Errorf("got error %q, want %q", m.gotError, m.wantError)
	}

	if m.gotFatal != m.wantFatal {
		t.Errorf("got error %v, want %v", m.gotFatal, m.wantFatal)
	}
}

// mock provides the same interface as testing.TB with overridden Errorf, Fatal
// and Heleper methods.
type mock struct {
	testing.TB
	isHelper  bool
	gotError  string
	wantError string
	gotFatal  string
	wantFatal string
}

func (m *mock) Helper() {
	m.isHelper = true
}

func (m *mock) Errorf(format string, args ...interface{}) {
	m.gotError = fmt.Sprintf(format, args...)
}

func (m *mock) Fatal(args ...interface{}) {
	m.gotFatal = fmt.Sprint(args...)
	panic(errFailed) // terminate the goroutine to detect it in the assert function
}

var errFailed = errors.New("failed")

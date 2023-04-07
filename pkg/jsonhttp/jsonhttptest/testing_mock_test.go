// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest_test

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

type testResult struct {
	errors []string
	fatal  string
}

// assert is a test helper that validates a functionality of another helper
// function by mocking Errorf, Fatal and Helper methods on testing.TB.
func assert(t *testing.T, want testResult, f func(m *mock)) {
	t.Helper()

	defer func() {
		if v := recover(); v != nil {
			if err, ok := v.(error); ok && errors.Is(err, errFailed) {
				return // execution of the goroutine is stopped by a mock Fatal function
			}
			t.Fatalf("panic: %v", v)
		}
	}()

	m := &mock{}

	f(m)

	if !m.isHelper { // Request function is tested and it must be always a helper
		t.Error("not a helper function")
	}

	gotErrors := sort.StringSlice(m.got.errors)
	wantErrors := sort.StringSlice(want.errors)
	if !reflect.DeepEqual(gotErrors, wantErrors) {
		t.Errorf("errors not as expected: got  %v, want %v", gotErrors, wantErrors)
	}

	if m.got.fatal != want.fatal {
		t.Errorf("got error %v, want %v", m.got.fatal, want.fatal)
	}
}

// mock provides the same interface as testing.TB with overridden Errorf, Fatal
// and Heleper methods.
type mock struct {
	testing.TB
	isHelper bool
	got      testResult
}

func (m *mock) Helper() {
	m.isHelper = true
}

func (m *mock) Errorf(format string, args ...interface{}) {
	m.got.errors = append(m.got.errors, fmt.Sprintf(format, args...))
}

func (m *mock) Fatal(args ...interface{}) {
	m.got.fatal = fmt.Sprint(args...)
	panic(errFailed) // terminate the goroutine to detect it in the assert function
}

var errFailed = errors.New("failed")

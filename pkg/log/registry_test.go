// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// allVerbosityLevels enumerates all possible verbosity levels.
var allVerbosityLevels = []Level{
	VerbosityNone,
	VerbosityError,
	VerbosityWarning,
	VerbosityInfo,
	VerbosityDebug,
	VerbosityAll,
}

// opts specifies different set options for testing.
var opts = []Option{
	WithSink(new(bytes.Buffer)),
	WithVerbosity(1),
	WithCaller(CategoryError),
	WithCallerFunc(),
	WithTimestamp(),
	WithTimestampLayout("0123456789"),
	WithMaxDepth(2),
	WithJSONOutput(),
	WithCallerDepth(3),
}

func TestModifyDefaults(t *testing.T) {
	o := defaults.options
	t.Cleanup(func() {
		defaults.pin = sync.Once{}
		defaults.options = o
	})

	want := new(Options)
	have := new(Options)
	for _, o := range opts {
		o(want)
	}
	defaults.options = have
	ModifyDefaults(opts...)

	diff := cmp.Diff(have, want, cmp.AllowUnexported(
		Options{},
		fmtOptions{},
		bytes.Buffer{},
	))
	if diff != "" {
		t.Errorf("ModifyDefaults(...) mismatch (-want +have):\n%s", diff)
	}
}

func TestNewLogger(t *testing.T) {
	l, o := loggers, defaults.options
	t.Cleanup(func() {
		loggers = l
		defaults.pin = sync.Once{}
		defaults.options = o
	})

	loggers = new(sync.Map)
	defaults.options = new(Options)
	ModifyDefaults(opts...)

	var (
		cnt int
		val interface{}
	)
	NewLogger("root").Register()
	loggers.Range(func(k, v interface{}) bool {
		cnt++
		val = v
		return true
	})
	if cnt != 1 {
		t.Fatalf("NewLogger(...) instance(s) count mismatch: want: 1; have: %d", cnt)
	}
	want := val.(*logger)
	have := NewLogger("root").Register()

	diff := cmp.Diff(have, want, cmp.AllowUnexported(
		logger{},
		caller{},
		formatter{},
		fmtOptions{},
		bytes.Buffer{},
	))
	if diff != "" {
		t.Errorf("NewLogger(...) mismatch (-want +have):\n%s", diff)
	}
}

func TestSetVerbosity(t *testing.T) {
	l, o := loggers, defaults.options
	t.Cleanup(func() {
		loggers = l
		defaults.pin = sync.Once{}
		defaults.options = o
	})

	loggers = new(sync.Map)
	defaults.options = new(Options)
	ModifyDefaults(opts...)

	NewLogger("root").Register()
	NewLogger("root").WithName("child1").Register()
	NewLogger("root").WithName("child1").WithValues("abc", 123).Register()

	registered := make(map[string]*logger)
	loggers.Range(func(k, v interface{}) bool {
		registered[k.(string)] = v.(*logger)
		return true
	})

	for _, verbosity := range allVerbosityLevels {
		t.Run(fmt.Sprintf("to=%s/by logger", verbosity), func(t *testing.T) {
			for _, logger := range registered {
				if err := SetVerbosity(logger, verbosity); err != nil {
					t.Errorf("SetVerbosity(...) unexpected error %v", err)
				}

				// On VerbosityAll, the maximum verbosity will
				// be set instead (VerbosityDebug in our case).
				if verbosity == VerbosityAll {
					verbosity = VerbosityDebug
				}

				want, have := verbosity, registered[logger.id].verbosity.get()
				if want != have {
					t.Errorf("SetVerbosity(...) want verbosity: %q; have: %q", want, have)
				}

			}
		})
	}

	for _, verbosity := range allVerbosityLevels {
		t.Run(fmt.Sprintf("to=%s/by exp", verbosity), func(t *testing.T) {
			if err := SetVerbosityByExp("^root", verbosity); err != nil {
				t.Errorf("SetVerbosityByExp(...) unexpected error %v", err)
			}

			// On VerbosityAll, the maximum verbosity will
			// be set instead (VerbosityDebug in our case).
			if verbosity == VerbosityAll {
				verbosity = VerbosityDebug
			}

			for _, logger := range registered {
				want, have := verbosity, logger.verbosity.get()
				if want != have {
					t.Errorf("SetVerbosityByExp(...) want verbosity: %q; have: %q", want, have)
				}
			}
		})
	}
}

func TestRegistryRange(t *testing.T) {
	l, o := loggers, defaults.options
	t.Cleanup(func() {
		loggers = l
		defaults.pin = sync.Once{}
		defaults.options = o
	})

	loggers = new(sync.Map)
	defaults.options = new(Options)
	ModifyDefaults(opts...)

	NewLogger("root").Register()
	NewLogger("root").WithName("child1").Register()
	NewLogger("root").WithName("child1").WithValues("abc", 123).Register()

	registered := make(map[string]*logger)
	loggers.Range(func(k, v interface{}) bool {
		registered[k.(string)] = v.(*logger)
		return true
	})

	var cnt int
	RegistryIterate(func(id, path string, verbosity Level, v uint) (next bool) {
		cnt++

		have := registered[id]
		if have.id != id {
			t.Errorf("RegistryIterate(...) want id: %q; have: %q", id, have.id)
		}
		if have.namesStr != path {
			t.Errorf("RegistryIterate(...) want namesStr: %q; have: %q", path, have.namesStr)
		}
		if have.verbosity.get() != verbosity {
			t.Errorf("RegistryIterate(...) want verbosity: %q; have: %q", verbosity, have.verbosity)
		}
		if have.v != v {
			t.Errorf("RegistryIterate(...) want: v %d; have: %d", v, have.v)
		}
		return true
	})

	if have, want := cnt, len(registered); have != want {
		t.Fatalf("RegistryIterate(...) instance(s) count mismatch: want: %d; have: %d", want, have)
	}
}

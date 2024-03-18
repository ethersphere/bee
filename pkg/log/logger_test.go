// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// hook is a helper type for recording
// if Hook.Fire method has been triggered.
type hook bool

// reset resets this hook as if it were never fired.
func (h *hook) reset() {
	*h = false
}

// Fire implements Hook.Fire method.
func (h *hook) Fire(Level) error {
	*h = true
	return nil
}

// applyError is a higher order function that returns the given fn with an applied err.
func applyError(fn func(error, string, ...interface{}), err error) func(string, ...interface{}) {
	return func(msg string, kvs ...interface{}) {
		fn(err, msg, kvs...)
	}
}

// newLogger is a convenient constructor for creating
// new instances of Logger without pinning the defaults.
func newLogger(modifications ...Option) (Builder, *bytes.Buffer) {
	const name = "root"

	opts := *defaults.options
	for _, modify := range modifications {
		modify(&opts)
	}

	b := new(bytes.Buffer)
	l := &logger{
		formatter:  newFormatter(opts.fmtOptions),
		verbosity:  opts.verbosity,
		sink:       b,
		levelHooks: opts.levelHooks,
	}
	l.builder = &builder{
		l:        l,
		names:    []string{name},
		namesStr: name,
	}
	return l, b
}

func TestLoggerOptionsLevelHooks(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	t.Run("verbosity=none", func(t *testing.T) {
		have := hook(false)
		logger, _ := newLogger(WithLevelHooks(VerbosityNone, &have))

		tests := []struct {
			fn   func(string, ...interface{})
			want bool
		}{{
			fn:   logger.Build().Debug,
			want: false,
		}, {
			fn:   logger.Build().Info,
			want: false,
		}, {
			fn:   logger.Build().Warning,
			want: false,
		}, {
			fn:   applyError(logger.Build().Error, nil),
			want: false,
		}}

		for _, tc := range tests {
			tc.fn("")
			if tc.want != bool(have) {
				t.Errorf("hook: want fired %t; have %t", tc.want, have)
			}
			have.reset()
		}
	})
	t.Run("verbosity=debug", func(t *testing.T) {
		have := hook(false)
		logger, _ := newLogger(WithLevelHooks(VerbosityDebug, &have))

		tests := []struct {
			fn   func(string, ...interface{})
			want bool
		}{{
			fn:   logger.Build().Debug,
			want: true,
		}, {
			fn:   logger.Build().Info,
			want: false,
		}, {
			fn:   logger.Build().Warning,
			want: false,
		}, {
			fn:   applyError(logger.Build().Error, nil),
			want: false,
		}}

		for _, tc := range tests {
			tc.fn("")
			if tc.want != bool(have) {
				t.Errorf("hook: want fired %t; have %t", tc.want, have)
			}
			have.reset()
		}
	})
	t.Run("verbosity=info", func(t *testing.T) {
		have := hook(false)
		logger, _ := newLogger(WithLevelHooks(VerbosityInfo, &have))

		tests := []struct {
			fn   func(string, ...interface{})
			want bool
		}{{
			fn:   logger.Build().Debug,
			want: false,
		}, {
			fn:   logger.Build().Info,
			want: true,
		}, {
			fn:   logger.Build().Warning,
			want: false,
		}, {
			fn:   applyError(logger.Build().Error, nil),
			want: false,
		}}

		for _, tc := range tests {
			tc.fn("")
			if tc.want != bool(have) {
				t.Errorf("hook: want fired %t; have %t", tc.want, have)
			}
			have.reset()
		}
	})
	t.Run("verbosity=warning", func(t *testing.T) {
		have := hook(false)
		logger, _ := newLogger(WithLevelHooks(VerbosityWarning, &have))

		tests := []struct {
			fn   func(string, ...interface{})
			want bool
		}{{
			fn:   logger.Build().Debug,
			want: false,
		}, {
			fn:   logger.Build().Info,
			want: false,
		}, {
			fn:   logger.Build().Warning,
			want: true,
		}, {
			fn:   applyError(logger.Build().Error, nil),
			want: false,
		}}

		for _, tc := range tests {
			tc.fn("")
			if tc.want != bool(have) {
				t.Errorf("hook: want fired %t; have %t", tc.want, have)
			}
			have.reset()
		}
	})
	t.Run("verbosity=error", func(t *testing.T) {
		have := hook(false)
		logger, _ := newLogger(WithLevelHooks(VerbosityError, &have))

		tests := []struct {
			fn   func(string, ...interface{})
			want bool
		}{{
			fn:   logger.Build().Debug,
			want: false,
		}, {
			fn:   logger.Build().Info,
			want: false,
		}, {
			fn:   logger.Build().Warning,
			want: false,
		}, {
			fn:   applyError(logger.Build().Error, nil),
			want: true,
		}}

		for _, tc := range tests {
			tc.fn("")
			if tc.want != bool(have) {
				t.Errorf("hook: want fired %t; have %t", tc.want, have)
			}
			have.reset()
		}
	})
	t.Run("verbosity=all", func(t *testing.T) {
		have := hook(false)
		logger, _ := newLogger(WithLevelHooks(VerbosityAll, &have))

		tests := []struct {
			fn   func(string, ...interface{})
			want bool
		}{{
			fn:   logger.Build().Debug,
			want: true,
		}, {
			fn:   logger.Build().Info,
			want: true,
		}, {
			fn:   logger.Build().Warning,
			want: true,
		}, {
			fn:   applyError(logger.Build().Error, nil),
			want: true,
		}}

		for _, tc := range tests {
			tc.fn("")
			if tc.want != bool(have) {
				t.Errorf("hook: want fired %t; have %t", tc.want, have)
			}
			have.reset()
		}
	})
}

func TestLoggerOptionsTimestampFormat(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	logger, bb := newLogger(
		WithTimestamp(),
		WithTimestampLayout("TIMESTAMP"),
		WithCallerDepth(1),
	)
	logger.Build().Info("msg")
	have := string(bytes.TrimRight(bb.Bytes(), "\n"))
	want := `"time"="TIMESTAMP" "level"="info" "logger"="root" "msg"="msg"`
	if have != want {
		t.Errorf("\nwant %q\nhave %q", want, have)
	}
}

func TestLogger(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	logger, bb := newLogger()

	testCases := []struct {
		name  string
		logFn func(string, ...interface{})
		args  []interface{}
		want  string
	}{{
		name:  "just msg",
		logFn: logger.Build().Debug,
		args:  makeKV(),
		want:  `"level"="debug" "logger"="root" "msg"="msg"`,
	}, {
		name:  "primitives",
		logFn: logger.Build().Debug,
		args:  makeKV("int", 1, "str", "ABC", "bool", true),
		want:  `"level"="debug" "logger"="root" "msg"="msg" "int"=1 "str"="ABC" "bool"=true`,
	}, {
		name:  "just msg",
		logFn: logger.Build().Info,
		args:  makeKV(),
		want:  `"level"="info" "logger"="root" "msg"="msg"`,
	}, {
		name:  "primitives",
		logFn: logger.Build().Info,
		args:  makeKV("int", 1, "str", "ABC", "bool", true),
		want:  `"level"="info" "logger"="root" "msg"="msg" "int"=1 "str"="ABC" "bool"=true`,
	}, {
		name:  "just msg",
		logFn: logger.Build().Warning,
		args:  makeKV(),
		want:  `"level"="warning" "logger"="root" "msg"="msg"`,
	}, {
		name:  "primitives",
		logFn: logger.Build().Warning,
		args:  makeKV("int", 1, "str", "ABC", "bool", true),
		want:  `"level"="warning" "logger"="root" "msg"="msg" "int"=1 "str"="ABC" "bool"=true`,
	}, {
		name:  "just msg",
		logFn: applyError(logger.Build().Error, errors.New("err")),
		args:  makeKV(),
		want:  `"level"="error" "logger"="root" "msg"="msg" "error"="err"`,
	}, {
		name:  "primitives",
		logFn: applyError(logger.Build().Error, errors.New("err")),
		args:  makeKV("int", 1, "str", "ABC", "bool", true),
		want:  `"level"="error" "logger"="root" "msg"="msg" "error"="err" "int"=1 "str"="ABC" "bool"=true`,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bb.Reset()
			tc.logFn("msg", tc.args...)
			have := string(bytes.TrimRight(bb.Bytes(), "\n"))
			if have != tc.want {
				t.Errorf("\nwant %q\nhave %q", tc.want, have)
			}
		})
	}
}

func TestLoggerWithCaller(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	t.Run("caller=CategoryAll", func(t *testing.T) {
		logger, bb := newLogger(WithCaller(CategoryAll))

		logger.Build().Debug("msg")
		_, file, line, _ := runtime.Caller(0)
		want := fmt.Sprintf(`"level"="debug" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line-1)
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		_, file, line, _ = runtime.Caller(0)
		want = fmt.Sprintf(`"level"="info" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line-1)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		_, file, line, _ = runtime.Caller(0)
		want = fmt.Sprintf(`"level"="warning" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line-1)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(errors.New("err"), "msg")
		_, file, line, _ = runtime.Caller(0)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = fmt.Sprintf(`"level"="error" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg" "error"="err"`, filepath.Base(file), line-1)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("caller=CategoryAll, logCallerFunc=true", func(t *testing.T) {
		const thisFunc = "github.com/ethersphere/bee/v2/pkg/log.TestLoggerWithCaller.func3"

		logger, bb := newLogger(WithCaller(CategoryAll), WithCallerFunc())

		logger.Build().Debug("msg")
		_, file, line, _ := runtime.Caller(0)
		want := fmt.Sprintf(`"level"="debug" "logger"="root" "caller"={"file":%q,"line":%d,"function":%q} "msg"="msg"`, filepath.Base(file), line-1, thisFunc)
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		_, file, line, _ = runtime.Caller(0)
		want = fmt.Sprintf(`"level"="info" "logger"="root" "caller"={"file":%q,"line":%d,"function":%q} "msg"="msg"`, filepath.Base(file), line-1, thisFunc)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		_, file, line, _ = runtime.Caller(0)
		want = fmt.Sprintf(`"level"="warning" "logger"="root" "caller"={"file":%q,"line":%d,"function":%q} "msg"="msg"`, filepath.Base(file), line-1, thisFunc)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(errors.New("err"), "msg")
		_, file, line, _ = runtime.Caller(0)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = fmt.Sprintf(`"level"="error" "logger"="root" "caller"={"file":%q,"line":%d,"function":%q} "msg"="msg" "error"="err"`, filepath.Base(file), line-1, thisFunc)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("caller=CategoryDebug", func(t *testing.T) {
		logger, bb := newLogger(WithCaller(CategoryDebug))

		logger.Build().Debug("msg")
		_, file, line, _ := runtime.Caller(0)
		want := fmt.Sprintf(`"level"="debug" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line-1)
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		want = `"level"="info" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(errors.New("err"), "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg" "error"="err"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("caller=CategoryInfo", func(t *testing.T) {
		logger, bb := newLogger(WithCaller(CategoryInfo))

		logger.Build().Debug("msg")
		want := `"level"="debug" "logger"="root" "msg"="msg"`
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		_, file, line, _ := runtime.Caller(0)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = fmt.Sprintf(`"level"="info" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line-1)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(errors.New("err"), "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg" "error"="err"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("caller=CategoryWarning", func(t *testing.T) {
		logger, bb := newLogger(WithCaller(CategoryWarning))

		logger.Build().Debug("msg")
		want := `"level"="debug" "logger"="root" "msg"="msg"`
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="info" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		_, file, line, _ := runtime.Caller(0)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = fmt.Sprintf(`"level"="warning" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line-1)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(errors.New("err"), "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg" "error"="err"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("caller=CategoryError", func(t *testing.T) {
		logger, bb := newLogger(WithCaller(CategoryError))

		logger.Build().Debug("msg")
		want := `"level"="debug" "logger"="root" "msg"="msg"`
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="info" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(errors.New("err"), "msg")
		_, file, line, _ := runtime.Caller(0)
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = fmt.Sprintf(`"level"="error" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg" "error"="err"`, filepath.Base(file), line-1)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("caller=CategoryNone", func(t *testing.T) {
		logger, bb := newLogger(WithCaller(CategoryNone))

		logger.Build().Debug("msg")
		want := `"level"="debug" "logger"="root" "msg"="msg"`
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="info" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(errors.New("err"), "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg" "error"="err"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
}

func TestLoggerWithName(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	logger, bb := newLogger()
	logger.Build()

	testCases := []struct {
		name  string
		logFn func(string, ...interface{})
		args  []interface{}
		want  string
	}{{
		name:  "one",
		logFn: logger.WithName("pfx1").Build().Debug,
		args:  makeKV("k", "v"),
		want:  `"level"="debug" "logger"="root/pfx1" "msg"="msg" "k"="v"`,
	}, {
		name:  "two",
		logFn: logger.WithName("pfx1").WithName("pfx2").Build().Debug,
		args:  makeKV("k", "v"),
		want:  `"level"="debug" "logger"="root/pfx1/pfx2" "msg"="msg" "k"="v"`,
	}, {
		name:  "one",
		logFn: logger.WithName("pfx1").Build().Info,
		args:  makeKV("k", "v"),
		want:  `"level"="info" "logger"="root/pfx1" "msg"="msg" "k"="v"`,
	}, {
		name:  "two",
		logFn: logger.WithName("pfx1").WithName("pfx2").Build().Info,
		args:  makeKV("k", "v"),
		want:  `"level"="info" "logger"="root/pfx1/pfx2" "msg"="msg" "k"="v"`,
	}, {
		name:  "one",
		logFn: logger.WithName("pfx1").Build().Warning,
		args:  makeKV("k", "v"),
		want:  `"level"="warning" "logger"="root/pfx1" "msg"="msg" "k"="v"`,
	}, {
		name:  "two",
		logFn: logger.WithName("pfx1").WithName("pfx2").Build().Warning,
		args:  makeKV("k", "v"),
		want:  `"level"="warning" "logger"="root/pfx1/pfx2" "msg"="msg" "k"="v"`,
	}, {
		name:  "one",
		logFn: applyError(logger.WithName("pfx1").Build().Error, errors.New("err")),
		args:  makeKV("k", "v"),
		want:  `"level"="error" "logger"="root/pfx1" "msg"="msg" "error"="err" "k"="v"`,
	}, {
		name:  "two",
		logFn: applyError(logger.WithName("pfx1").WithName("pfx2").Build().Error, errors.New("err")),
		args:  makeKV("k", "v"),
		want:  `"level"="error" "logger"="root/pfx1/pfx2" "msg"="msg" "error"="err" "k"="v"`,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bb.Reset()
			tc.logFn("msg", tc.args...)
			have := string(bytes.TrimRight(bb.Bytes(), "\n"))
			if have != tc.want {
				t.Errorf("\nwant %q\nhave %q", tc.want, have)
			}
		})
	}
}

func TestLoggerWithValues(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	logger, bb := newLogger()

	testCases := []struct {
		name  string
		logFn func(string, ...interface{})
		args  []interface{}
		want  string
	}{{
		name:  "zero",
		logFn: logger.Build().Debug,
		args:  makeKV("k", "v"),
		want:  `"level"="debug" "logger"="root" "msg"="msg" "k"="v"`,
	}, {
		name:  "one",
		logFn: logger.WithValues("one", 1).Build().Debug,
		args:  makeKV("k", "v"),
		want:  `"level"="debug" "logger"="root" "msg"="msg" "one"=1 "k"="v"`,
	}, {
		name:  "two",
		logFn: logger.WithValues("one", 1, "two", 2).Build().Debug,
		args:  makeKV("k", "v"),
		want:  `"level"="debug" "logger"="root" "msg"="msg" "one"=1 "two"=2 "k"="v"`,
	}, {
		name:  "dangling",
		logFn: logger.WithValues("dangling").Build().Debug,
		args:  makeKV("k", "v"),
		want:  `"level"="debug" "logger"="root" "msg"="msg" "dangling"="<no-value>" "k"="v"`,
	}, {
		name:  "zero",
		logFn: logger.Build().Info,
		args:  makeKV("k", "v"),
		want:  `"level"="info" "logger"="root" "msg"="msg" "k"="v"`,
	}, {
		name:  "one",
		logFn: logger.WithValues("one", 1).Build().Info,
		args:  makeKV("k", "v"),
		want:  `"level"="info" "logger"="root" "msg"="msg" "one"=1 "k"="v"`,
	}, {
		name:  "two",
		logFn: logger.WithValues("one", 1, "two", 2).Build().Info,
		args:  makeKV("k", "v"),
		want:  `"level"="info" "logger"="root" "msg"="msg" "one"=1 "two"=2 "k"="v"`,
	}, {
		name:  "dangling",
		logFn: logger.WithValues("dangling").Build().Info,
		args:  makeKV("k", "v"),
		want:  `"level"="info" "logger"="root" "msg"="msg" "dangling"="<no-value>" "k"="v"`,
	}, {
		name:  "zero",
		logFn: logger.Build().Warning,
		args:  makeKV("k", "v"),
		want:  `"level"="warning" "logger"="root" "msg"="msg" "k"="v"`,
	}, {
		name:  "one",
		logFn: logger.WithValues("one", 1).Build().Warning,
		args:  makeKV("k", "v"),
		want:  `"level"="warning" "logger"="root" "msg"="msg" "one"=1 "k"="v"`,
	}, {
		name:  "two",
		logFn: logger.WithValues("one", 1, "two", 2).Build().Warning,
		args:  makeKV("k", "v"),
		want:  `"level"="warning" "logger"="root" "msg"="msg" "one"=1 "two"=2 "k"="v"`,
	}, {
		name:  "dangling",
		logFn: logger.WithValues("dangling").Build().Warning,
		args:  makeKV("k", "v"),
		want:  `"level"="warning" "logger"="root" "msg"="msg" "dangling"="<no-value>" "k"="v"`,
	}, {
		name:  "zero",
		logFn: applyError(logger.Build().Error, errors.New("err")),
		args:  makeKV("k", "v"),
		want:  `"level"="error" "logger"="root" "msg"="msg" "error"="err" "k"="v"`,
	}, {
		name:  "one",
		logFn: applyError(logger.WithValues("one", 1).Build().Error, errors.New("err")),
		args:  makeKV("k", "v"),
		want:  `"level"="error" "logger"="root" "msg"="msg" "error"="err" "one"=1 "k"="v"`,
	}, {
		name:  "two",
		logFn: applyError(logger.WithValues("one", 1, "two", 2).Build().Error, errors.New("err")),
		args:  makeKV("k", "v"),
		want:  `"level"="error" "logger"="root" "msg"="msg" "error"="err" "one"=1 "two"=2 "k"="v"`,
	}, {
		name:  "dangling",
		logFn: applyError(logger.WithValues("dangling").Build().Error, errors.New("err")),
		args:  makeKV("k", "v"),
		want:  `"level"="error" "logger"="root" "msg"="msg" "error"="err" "dangling"="<no-value>" "k"="v"`,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bb.Reset()
			tc.logFn("msg", tc.args...)
			have := string(bytes.TrimRight(bb.Bytes(), "\n"))
			if have != tc.want {
				t.Errorf("\nwant %q\nhave %q", tc.want, have)
			}
		})
	}
}

func TestLoggerWithCallDepth(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	t.Run("verbosity=debug, callerDepth=1", func(t *testing.T) {
		logger, bb := newLogger(WithCallerDepth(1), WithCaller(CategoryAll))
		logger.Build().Debug("msg")
		_, file, line, _ := runtime.Caller(1)
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		want := fmt.Sprintf(`"level"="debug" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=info, callerDepth=1", func(t *testing.T) {
		logger, bb := newLogger(WithCallerDepth(1), WithCaller(CategoryAll))
		logger.Build().Info("msg")
		_, file, line, _ := runtime.Caller(1)
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		want := fmt.Sprintf(`"level"="info" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=warning, callerDepth=1", func(t *testing.T) {
		logger, bb := newLogger(WithCallerDepth(1), WithCaller(CategoryAll))
		logger.Build().Warning("msg")
		_, file, line, _ := runtime.Caller(1)
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		want := fmt.Sprintf(`"level"="warning" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg"`, filepath.Base(file), line)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=error, callerDepth=1", func(t *testing.T) {
		logger, bb := newLogger(WithCallerDepth(1), WithCaller(CategoryAll))
		logger.Build().Error(errors.New("err"), "msg")
		_, file, line, _ := runtime.Caller(1)
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		want := fmt.Sprintf(`"level"="error" "logger"="root" "caller"={"file":%q,"line":%d} "msg"="msg" "error"="err"`, filepath.Base(file), line)
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
}

func TestLoggerVerbosity(t *testing.T) {
	l := loggers
	t.Cleanup(func() {
		loggers = l
	})
	loggers = new(sync.Map)

	t.Run("verbosity=all", func(t *testing.T) {
		logger, bb := newLogger(WithVerbosity(VerbosityAll))
		logger.Build()

		logger.V(1).Build().Debug("msg")
		want := `"level"="debug" "logger"="root" "v"=1 "msg"="msg"`
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Debug("msg")
		want = `"level"="debug" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		want = `"level"="info" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(nil, "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=debug", func(t *testing.T) {
		logger, bb := newLogger(WithVerbosity(VerbosityDebug))
		logger.Build()

		logger.V(1).Build().Debug("msg")
		want := ""
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Debug("msg")
		want = `"level"="debug" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		want = `"level"="info" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(nil, "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=info", func(t *testing.T) {
		logger, bb := newLogger(WithVerbosity(VerbosityInfo))
		logger.Build()

		logger.V(1).Build().Debug("msg")
		logger.Build().Debug("msg")
		want := ""
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Info("msg")
		want = `"level"="info" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(nil, "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=warning", func(t *testing.T) {
		logger, bb := newLogger(WithVerbosity(VerbosityWarning))
		logger.Build()

		logger.V(1).Build().Debug("msg")
		logger.Build().Debug("msg")
		logger.Build().Info("msg")
		want := ""
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Warning("msg")
		want = `"level"="warning" "logger"="root" "msg"="msg"`
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(nil, "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=error", func(t *testing.T) {
		logger, bb := newLogger(WithVerbosity(VerbosityError))
		logger.Build()

		logger.V(1).Build().Debug("msg")
		logger.Build().Debug("msg")
		logger.Build().Info("msg")
		logger.Build().Warning("msg")
		want := ""
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}

		bb.Reset()

		logger.Build().Error(nil, "msg")
		have = string(bytes.TrimRight(bb.Bytes(), "\n"))
		want = `"level"="error" "logger"="root" "msg"="msg"`
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
	t.Run("verbosity=none", func(t *testing.T) {
		logger, bb := newLogger(WithVerbosity(VerbosityNone))
		logger.Build()

		logger.V(1).Build().Debug("msg")
		logger.Build().Debug("msg")
		logger.Build().Info("msg")
		logger.Build().Warning("msg")
		logger.Build().Error(nil, "msg")
		want := ""
		have := string(bytes.TrimRight(bb.Bytes(), "\n"))
		if have != want {
			t.Errorf("\nwant %q\nhave %q", want, have)
		}
	})
}

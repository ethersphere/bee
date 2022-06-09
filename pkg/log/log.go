// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

// Level specifies a level of verbosity for logger.
// Level should be modified only through its set method.
// Level is treated as a sync/atomic int32.
type Level int32

// get returns the value of the Level.
func (l *Level) get() Level {
	return Level(atomic.LoadInt32((*int32)(l)))
}

// set updates the value of the Level.
func (l *Level) set(v Level) {
	atomic.StoreInt32((*int32)(l), int32(v))
}

// String implements the fmt.Stringer interface.
func (l Level) String() string {
	switch l.get() {
	case VerbosityNone:
		return "none"
	case VerbosityError:
		return "error"
	case VerbosityWarning:
		return "warning"
	case VerbosityInfo:
		return "info"
	case VerbosityDebug:
		return "debug"
	case VerbosityAll:
		return "all"
	}
	return strconv.FormatInt(int64(l), 10) // Covers all in the range [VerbosityDebug ... VerbosityAll>.
}

// ParseVerbosityLevel returns a verbosity Level parsed from the given s.
func ParseVerbosityLevel(s string) (Level, error) {
	switch s {
	case "none":
		return VerbosityNone, nil
	case "error":
		return VerbosityError, nil
	case "warning":
		return VerbosityWarning, nil
	case "info":
		return VerbosityInfo, nil
	case "debug":
		return VerbosityDebug, nil
	case "all":
		return VerbosityAll, nil
	}
	i, err := strconv.ParseInt(s, 10, 32)
	return Level(i), err
}

const (
	// VerbosityNone will silence the logger.
	VerbosityNone = Level(iota - 4)
	// VerbosityError allows only error messages to be printed.
	VerbosityError
	// VerbosityWarning allows only error and warning messages to be printed.
	VerbosityWarning
	// VerbosityInfo allows only error, warning and info messages to be printed.
	VerbosityInfo
	// VerbosityDebug allows only error, warning, info and debug messages to be printed.
	VerbosityDebug
	// VerbosityAll allows to print all messages up to and including level V.
	// It is a placeholder for the maximum possible V level verbosity within the logger.
	VerbosityAll = Level(1<<31 - 1)
)

// Hook that is fired when logging
// on the associated severity log level.
// Note, the call must be non-blocking.
type Hook interface {
	Fire(Level) error
}

// Builder specifies a set of methods that can be used to
// modify the behavior of the logger before it is created.
type Builder interface {
	// V specifies verbosity level added to the debug verbosity, relative to
	// the parent Logger. In other words, V-levels are additive. A higher
	// verbosity level means a log message is less important.
	V(v uint) Builder

	// WithName specifies a name element added to the Logger's name.
	// Successive calls with WithName append additional suffixes to the
	// Logger's name. It's strongly recommended that name segments contain
	// only letters, digits, and hyphens (see the package documentation for
	// more information). Formatter uses '/' characters to separate name
	// elements. Callers should not pass '/' in the provided name string.
	WithName(name string) Builder

	// WithValues specifies additional key/value pairs
	// to be logged with each log line.
	WithValues(keysAndValues ...interface{}) Builder

	// Build returns new or existing Logger
	// instance, if such instance already exists.
	Build() Logger
}

// Logger provides a set of methods that define the behavior of the logger.
type Logger interface {
	Builder

	// Debug logs a debug message with the given key/value pairs as context.
	// The msg argument should be used to add some constant description to
	// the log line. The key/value pairs can then be used to add additional
	// variable information. The key/value pairs must alternate string keys
	// and arbitrary values.
	Debug(msg string, keysAndValues ...interface{})

	// Info logs an info message with the given key/value pairs as context.
	// The msg argument should be used to add some constant description to
	// the log line. The key/value pairs can then be used to add additional
	// variable information. The key/value pairs must alternate string keys
	// and arbitrary values.
	Info(msg string, keysAndValues ...interface{})

	// Warning logs a warning message with the given key/value pairs as context.
	// The msg argument should be used to add some constant description to
	// the log line. The key/value pairs can then be used to add additional
	// variable information. The key/value pairs must alternate string keys
	// and arbitrary values.
	Warning(msg string, keysAndValues ...interface{})

	// Error logs an error, with the given message and key/value pairs as context.
	// The msg argument should be used to add context to any underlying error,
	// while the err argument should be used to attach the actual error that
	// triggered this log line, if present. The err parameter is optional
	// and nil may be passed instead of an error instance.
	Error(err error, msg string, keysAndValues ...interface{})
}

// Lock wraps io.Writer in a mutex to make it safe for concurrent use.
// In particular, *os.Files must be locked before use.
func Lock(w io.Writer) io.Writer {
	if _, ok := w.(*lockWriter); ok {
		return w // No need to layer on another lock.
	}
	return &lockWriter{w: w}
}

// lockWriter attaches mutex to io.Writer for convince of usage.
type lockWriter struct {
	sync.Mutex
	w io.Writer
}

// Write implements the io.Writer interface.
func (ls *lockWriter) Write(bs []byte) (int, error) {
	ls.Lock()
	n, err := ls.w.Write(bs)
	ls.Unlock()
	return n, err
}

// Options specifies parameters that affect logger behavior.
type Options struct {
	sink       io.Writer
	verbosity  Level
	levelHooks levelHooks
	fmtOptions fmtOptions
}

// Option represent Options parameters modifier.
type Option func(*Options)

// WithSink tells the logger to log to the given sync.
// The provided sync should be safe for concurrent use,
// if it is not then it should be wrapped with Lock helper.
func WithSink(sink io.Writer) Option {
	return func(opts *Options) { opts.sink = sink }
}

// WithVerbosity tells the logger which verbosity level should be logged by default.
func WithVerbosity(verbosity Level) Option {
	return func(opts *Options) { opts.verbosity = verbosity }
}

// WithCaller tells the logger to add a "caller" key to some or all log lines.
// This has some overhead, so some users might not want it.
func WithCaller(category MessageCategory) Option {
	return func(opts *Options) { opts.fmtOptions.caller = category }
}

// WithCallerFunc tells the logger to also log the calling function name.
// This has no effect if caller logging is not enabled (see WithCaller).
func WithCallerFunc() Option {
	return func(opts *Options) { opts.fmtOptions.logCallerFunc = true }
}

// WithTimestamp tells the logger to add a "timestamp" key to log lines.
// This has some overhead, so some users might not want it.
func WithTimestamp() Option {
	return func(opts *Options) { opts.fmtOptions.logTimestamp = true }
}

// WithTimestampLayout tells the logger how to render timestamps when
// WithTimestamp is enabled. If not specified, a default format will
// be used. For more details, see docs for Go's time.Layout.
func WithTimestampLayout(layout string) Option {
	return func(opts *Options) { opts.fmtOptions.timestampLayout = layout }
}

// WithMaxDepth tells the logger how many levels of nested fields
// (e.g. a struct that contains a struct, etc.) it may log. Every time
// it finds a struct, slice, array, or map the depth is increased by one.
// When the maximum is reached, the value will be converted to a string
// indicating that the max depth has been exceeded. If this field is not
// specified, a default value will be used.
func WithMaxDepth(depth int) Option {
	return func(opts *Options) { opts.fmtOptions.maxLogDepth = depth }
}

// WithJSONOutput tells the logger if the output should be formatted as JSON.
func WithJSONOutput() Option {
	return func(opts *Options) { opts.fmtOptions.jsonOutput = true }
}

// WithCallerDepth tells the logger the number of stack-frames
// to skip when attributing the log line to a file and line.
func WithCallerDepth(depth int) Option {
	return func(opts *Options) { opts.fmtOptions.callerDepth = depth }
}

// WithLevelHooks tells the logger to register and execute hooks at
// related severity log levels. If VerbosityAll is given, then the
// given hooks will be registered with each severity log level.
// On the other hand, if VerbosityNone is given, hooks will
// not be registered with any severity log level.
func WithLevelHooks(l Level, hooks ...Hook) Option {
	return func(opts *Options) {
		if opts.levelHooks == nil {
			opts.levelHooks = make(map[Level][]Hook)
		}
		switch l {
		case VerbosityNone:
			return
		case VerbosityAll:
			for _, ml := range []Level{
				VerbosityError,
				VerbosityWarning,
				VerbosityInfo,
				VerbosityDebug,
			} {
				opts.levelHooks[ml] = append(opts.levelHooks[ml], hooks...)
			}
		default:
			opts.levelHooks[l] = append(opts.levelHooks[l], hooks...)
		}

	}
}

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
)

var _ Logger = (*logger)(nil)

// levelHooks is a helper type for storing and
// help triggering the hooks on a logger instance.
type levelHooks map[Level][]Hook

// fire triggers all the hooks for the given level.
func (lh levelHooks) fire(level Level) error {
	for _, hook := range lh[level] {
		if err := hook.Fire(level); err != nil {
			return err
		}
	}
	return nil
}

// logger implements the Logger interface.
type logger struct {
	// id is the unique identifier of a logger.
	// It identifies the instance of a logger in the logger registry.
	id string

	// v level represents the granularity of debug calls.
	v uint

	// vc captures the original value of v before it is modified,
	// which can happen during the build of the logger.
	vc *uint

	// names represents a path in the tree,
	// element 0 is the root of the tree.
	names []string

	// namesStr is a cache of render names slice, so
	// we don't have to render them on each Build call.
	namesStr string

	// namesLen captures the original length of the names slice before new
	// elements are added, which can happen during the build of the logger.
	namesLen int

	// values holds additional key/value pairs
	// that are included on every log call.
	values []interface{}

	// valuesStr is a cache of render values slice, so
	// we don't have to render them on each Build call.
	valuesStr string

	// valuesLen captures the original length of the values slice before new
	// elements are added, which can happen during the build of the logger.
	valuesLen int

	// formatter formats log messages before they are written to the sink.
	formatter *formatter

	// verbosity represents the current verbosity level.
	// This variable is used to modify the verbosity of the logger instance.
	// Higher values enable more logs. Logs at or below this level
	// will be written, while logs above this level will be discarded.
	verbosity Level

	// sink represents the stream where the logs are written.
	sink io.Writer

	// levelHooks allow triggering of registered hooks
	// on their associated severity log levels.
	levelHooks levelHooks
}

// Debug implements the Logger interface Debug method.
func (l *logger) Debug(msg string, keysAndValues ...interface{}) {
	if int(l.verbosity.get()) >= int(l.v) {
		if err := l.log(VerbosityDebug, CategoryDebug, nil, msg, keysAndValues...); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

// Info implements the Logger interface Info method.
func (l *logger) Info(msg string, keysAndValues ...interface{}) {
	if l.verbosity.get() >= VerbosityInfo {
		if err := l.log(VerbosityInfo, CategoryInfo, nil, msg, keysAndValues...); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

// Warning implements the Logger interface Warning method.
func (l *logger) Warning(msg string, keysAndValues ...interface{}) {
	if l.verbosity.get() >= VerbosityWarning {
		if err := l.log(VerbosityWarning, CategoryWarning, nil, msg, keysAndValues...); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

// Error implements the Logger interface Error method.
func (l *logger) Error(err error, msg string, keysAndValues ...interface{}) {
	if l.verbosity.get() >= VerbosityError {
		if err := l.log(VerbosityError, CategoryError, err, msg, keysAndValues...); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

// V implements the Builder interface V method.
func (l *logger) V(level uint) Builder {
	if level > 0 {
		if l.vc == nil {
			v := l.v
			l.vc = &v
		}
		*l.vc += level
	}
	return l
}

// WithName implements the Builder interface WithName method.
func (l *logger) WithName(name string) Builder {
	l.names = append(l.names, name)
	return l
}

// WithValues implements the Builder interface WithValues method.
func (l *logger) WithValues(keysAndValues ...interface{}) Builder {
	l.values = append(l.values, keysAndValues...)
	return l
}

// Build implements the Builder interface Build method.
func (l *logger) Build() Logger {
	if l.id != "" &&
		l.vc == nil &&
		l.namesLen == len(l.names) &&
		l.valuesLen == len(l.values) {
		return l // Ignore subsequent chained calls to the Build method.
	}

	v := l.v
	if l.vc != nil {
		v = *l.vc
	}

	namesStr := l.namesStr
	if l.namesLen < len(l.names) {
		namesStr = strings.Join(l.names, "/")
	}

	valuesStr := l.valuesStr
	if l.valuesLen < len(l.values) {
		// ~5 is the average length of an English word; 4 is the rune size.
		len := nextPowOf2(uint64(5 * 4 * len(l.values)))
		buf := bytes.NewBuffer(make([]byte, 0, len))
		l.formatter.flatten(buf, l.values, false, false)
		valuesStr = buf.String()
	}

	// Basic builder invariants must be restored to the root
	// instance from which other instances are created.
	defer func() {
		l.vc = nil
		l.names = l.names[:l.namesLen]
		l.values = l.values[:l.valuesLen]
	}()

	key, val := hash(namesStr, v, valuesStr, l.sink), (*logger)(nil)
	switch i, ok := loggers.Load(key); {
	case ok: // Nothing to build, the instance exists.
		return i.(*logger)
	case l.id == "": // A new root instance.
		val = l
		val.id = key
		val.v = v
		val.namesStr = namesStr
		val.namesLen = len(val.names)
		val.valuesStr = valuesStr
		val.valuesLen = len(val.values)
	default: // A new child instance.
		c := *l
		val = &c
		val.id = key
		val.v = v
		val.namesStr = namesStr
		val.namesLen = len(val.names)
		val.names = append(make([]string, 0, val.namesLen), val.names...)
		val.valuesStr = valuesStr
		val.valuesLen = len(val.values)
		val.values = append(make([]interface{}, 0, val.valuesLen), val.values...)
	}
	return val
}

// Register implements the Builder interface Register method.
func (l *logger) Register() Logger {
	val := l.Build()
	key := hash(l.namesStr, l.v, l.valuesStr, l.sink)
	res, _ := loggers.LoadOrStore(key, val)
	return res.(*logger)
}

// setVerbosity changes the verbosity level or the logger.
func (l *logger) setVerbosity(v Level) {
	l.verbosity.set(v)
}

// log logs the given msg and key-value pairs with the given level
// and the given message category caller (if enabled) to the sink.
func (l *logger) log(vl Level, mc MessageCategory, err error, msg string, keysAndValues ...interface{}) error {
	base := make([]interface{}, 0, 14+len(keysAndValues))
	if l.formatter.opts.logTimestamp {
		base = append(base, "time", time.Now().Format(l.formatter.opts.timestampLayout))
	}
	base = append(base, "level", vl.String(), "logger", l.namesStr)
	if vl == VerbosityDebug && l.v > 0 {
		base = append(base, "v", l.v)
	}
	if policy := l.formatter.opts.caller; policy == CategoryAll || policy == mc {
		base = append(base, "caller", l.formatter.caller())
	}
	base = append(base, "msg", msg)
	if vl == VerbosityError {
		if err != nil {
			base = append(base, "error", err.Error())
		}
	}
	if len(l.values) > 0 {
		base = append(base, l.values...)
	}
	buf := l.formatter.render(base, keysAndValues)

	var merr *multierror.Error
	if _, err = l.sink.Write(buf); err != nil {
		merr = multierror.Append(
			merr,
			fmt.Errorf("log %s: failed to write message: %v\n", vl, err),
		)
	}
	if err := l.levelHooks.fire(vl); err != nil {
		merr = multierror.Append(
			merr,
			fmt.Errorf("log %s: failed to fire hooks: %v\n", vl, err),
		)
	}
	return merr.ErrorOrNil()
}

// hash is a hashing function for creating unique identifiers.
func hash(prefix string, v uint, values string, w io.Writer) string {
	var sink uintptr
	if reflect.ValueOf(w).Kind() == reflect.Ptr {
		sink = reflect.ValueOf(w).Pointer()
	} else {
		sink = reflect.ValueOf(&w).Pointer()
	}
	return fmt.Sprintf("%s[%d][%s]>>%d", prefix, v, values, sink)
}

// nextPowOf2 rounds up n to the next highest power of 2.
// See: https://graphics.stanford.edu/~seander/bithacks.html#RoundUpPowerOf2
func nextPowOf2(n uint64) uint64 {
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++
	return n
}

// TODO:
// - Implement the HTTP log middleware
// - Write benchmarks and do optimizations; consider `func (l *VLogger) getBuffer() *buffer` from glog

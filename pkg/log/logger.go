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

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/hashicorp/go-multierror"
	"github.com/prometheus/client_golang/prometheus"
)

var _ Logger = (*logger)(nil)

// levelHooks is a helper type for storing and
// help triggering the hooks on a logger instance.
type levelHooks map[Level][]Hook

// fire triggers all the hooks for the given level.
// If level V is enabled in debug verbosity, then
// the VerbosityAll hooks are triggered.
func (lh levelHooks) fire(level Level) error {
	if level > VerbosityDebug {
		level = VerbosityAll
	}
	for _, hook := range lh[level] {
		if err := hook.Fire(level); err != nil {
			return err
		}
	}
	return nil
}

type builder struct {
	l *logger

	// clone indicates whether this builder was cloned.
	cloned bool

	// v level represents the granularity of debug calls.
	v uint

	// names represents a path in the tree,
	// element 0 is the root of the tree.
	names []string

	// namesStr is a cache of render names slice, so
	// we don't have to render them on each Build call.
	namesStr string

	// values holds additional key/value pairs
	// that are included on every log call.
	values []interface{}

	// valuesStr is a cache of render values slice, so
	// we don't have to render them on each Build call.
	valuesStr string
}

// V implements the Builder interface V method.
func (b *builder) V(level uint) Builder {
	if level > 0 {
		c := b.clone()
		c.v += level
		return c
	}
	return b
}

// WithName implements the Builder interface WithName method.
func (b *builder) WithName(name string) Builder {
	c := b.clone()
	c.names = append(c.names, name)
	return c
}

// WithValues implements the Builder interface WithValues method.
func (b *builder) WithValues(keysAndValues ...interface{}) Builder {
	c := b.clone()
	c.values = append(c.values, keysAndValues...)
	return c
}

// Build implements the Builder interface Build method.
func (b *builder) Build() Logger {
	if !b.cloned && b.l.id != "" {
		return b.l
	}

	b.namesStr = strings.Join(b.names, "/")
	// ~5 is the average length of an English word; 4 is the rune size.
	bufCap := nextPowOf2(uint64(5 * 4 * len(b.values)))
	buf := bytes.NewBuffer(make([]byte, 0, bufCap))
	b.l.formatter.flatten(buf, b.values, false, false)
	b.valuesStr = buf.String()

	key := hash(b.namesStr, b.v, b.valuesStr, b.l.sink)
	if i, ok := loggers.Load(key); ok {
		// Nothing to build, the instance exists.
		return i.(*logger)
	}
	// A new child instance.
	c := *b.l
	b.l = &c
	c.builder = b
	c.cloned = false
	c.id = key

	return &c
}

// Register implements the Builder interface Register method.
func (b *builder) Register() Logger {
	val := b.Build()
	key := hash(b.namesStr, b.v, b.valuesStr, b.l.sink)
	res, _ := loggers.LoadOrStore(key, val)
	return res.(*logger)
}

func (b *builder) clone() *builder {
	if b.cloned {
		return b
	}

	c := *b
	c.cloned = true
	c.names = append(make([]string, 0, len(c.names)), c.names...)
	c.values = append(make([]interface{}, 0, len(c.values)), c.values...)
	return &c
}

// logger implements the Logger interface.
type logger struct {
	*builder

	// id is the unique identifier of a logger.
	// It identifies the instance of a logger in the logger registry.
	id string

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

	// metrics collects basic statistics about logged messages.
	metrics *metrics
}

// Metrics implements metrics.Collector interface.
func (l *logger) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(l.metrics)
}

// Verbosity implements the Logger interface Verbosity method.
func (l *logger) Verbosity() Level {
	return l.verbosity.get()
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
			fmt.Errorf("log %s: failed to write message: %w", vl, err),
		)
	}
	if err := l.levelHooks.fire(vl + Level(l.v)); err != nil {
		merr = multierror.Append(
			merr,
			fmt.Errorf("log %s: failed to fire hooks: %w", vl, err),
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

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import (
	"fmt"
	"os"
	"regexp"
	"sync"

	"github.com/hashicorp/go-multierror"
)

// defaults specifies the default global options for log
// package which every new logger will inherit on its creation.
var defaults = struct {
	pin       sync.Once // pin pins the options and formatter settings.
	options   *Options
	formatter *formatter
}{
	options: &Options{
		sink:      Lock(os.Stderr),
		verbosity: VerbosityDebug,
		fmtOptions: fmtOptions{
			timestampLayout: "2006-01-02 15:04:05.000000",
			maxLogDepth:     16,
		},
	},
}

// ModifyDefaults modifies the global default options for this log package
// that each new logger inherits when it is created. The default values can
// be modified only once, so further calls to this function will be ignored.
// This function should be called before the first call to the NewLogger
// factory constructor, otherwise it will have no effect.
func ModifyDefaults(opts ...Option) {
	defaults.pin.Do(func() {
		for _, modify := range opts {
			modify(defaults.options)
		}
		defaults.formatter = newFormatter(defaults.options.fmtOptions)
	})
}

// loggers is the central register for Logger instances.
var loggers = new(sync.Map)

// NewLogger is a factory constructor which returns a new logger instance
// based on the given name. If such an instance already exists in the
// logger registry, then this existing instance is returned instead.
func NewLogger(name string) Logger {
	// Pin the default settings if
	// they are not already pinned.
	ModifyDefaults()

	val, ok := loggers.Load(hash(name, 0, "", defaults.options.sink))
	if ok {
		return val.(*logger)
	}

	return &logger{
		names:      []string{name},
		namesStr:   name,
		namesLen:   1,
		formatter:  defaults.formatter,
		verbosity:  defaults.options.verbosity,
		sink:       defaults.options.sink,
		levelHooks: defaults.options.levelHooks,
	}
}

// SetVerbosity sets the level
// of verbosity of the given logger.
func SetVerbosity(l Logger, v Level) error {
	bl := l.(*logger)
	switch new, max := v.get(), Level(bl.v); {
	case new == VerbosityAll:
		bl.setVerbosity(max)
	case new > max:
		return fmt.Errorf("maximum verbosity %d exceeded for logger: %s", bl.v, bl.id)
	default:
		bl.setVerbosity(new)
	}
	return nil
}

// SetVerbosityByExp sets all loggers to the given
// verbosity level v that match the given expression
// e, which can be a logger id or a regular expression.
// An error is returned if e fails to compile.
func SetVerbosityByExp(e string, v Level) error {
	val, ok := loggers.Load(e)
	if ok {
		val.(*logger).setVerbosity(v)
		return nil
	}

	rex, err := regexp.Compile(e)
	if err != nil {
		return err
	}

	var merr *multierror.Error
	loggers.Range(func(key, val interface{}) bool {
		if rex.MatchString(key.(string)) {
			merr = multierror.Append(merr, SetVerbosity(val.(*logger), v))
		}
		return true
	})
	return merr.ErrorOrNil()
}

// RegistryIterate iterates through all registered loggers.
func RegistryIterate(fn func(id, path string, verbosity Level, v uint) (next bool)) {
	loggers.Range(func(_, val interface{}) bool {
		l := val.(*logger)
		return fn(l.id, l.namesStr, l.verbosity.get(), l.v)
	})
}

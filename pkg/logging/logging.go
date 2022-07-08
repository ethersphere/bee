// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package logging provides the logger interface abstraction
// and implementation for Bee. It uses logrus under the hood.
package logging

import (
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/sirupsen/logrus"
)

type Logger interface {
	Log(verbosity logrus.Level, args ...interface{})
	Tracef(format string, args ...interface{})
	Trace(args ...interface{})
	Debugf(format string, args ...interface{})
	Debug(args ...interface{})
	Infof(format string, args ...interface{})
	Info(args ...interface{})
	Warningf(format string, args ...interface{})
	Warning(args ...interface{})
	Errorf(format string, args ...interface{})
	Error(args ...interface{})
	WithValues(keysAndValues ...interface{}) Logger
}

type wrapper struct {
	noop    bool
	trace   log.Logger
	logger  log.Logger
	metrics metrics
}

func (w *wrapper) Log(verbosity logrus.Level, args ...interface{}) {
	if w.noop {
		return
	}
	switch verbosity {
	case logrus.DebugLevel:
		w.Debug(args...)
	case logrus.InfoLevel:
		w.Info(args...)
	case logrus.WarnLevel:
		w.Warning(args...)
	case logrus.ErrorLevel:
		w.Error(args...)
	default:
		w.trace.Debug(fmt.Sprint(args...))
	}
}

func (w *wrapper) Tracef(format string, args ...interface{}) {
	if !w.noop {
		w.trace.Debug(fmt.Sprintf(format, args...))
	}
}

func (w *wrapper) Trace(args ...interface{}) {
	if !w.noop {
		w.trace.Debug(fmt.Sprint(args...))
	}
}

func (w *wrapper) Debugf(format string, args ...interface{}) {
	if !w.noop {
		w.logger.Debug(fmt.Sprintf(format, args...))
	}
}

func (w *wrapper) Debug(args ...interface{}) {
	if !w.noop {
		w.logger.Debug(fmt.Sprint(args...))
	}
}

func (w *wrapper) Infof(format string, args ...interface{}) {
	if !w.noop {
		w.logger.Info(fmt.Sprintf(format, args...))
	}
}

func (w *wrapper) Info(args ...interface{}) {
	if !w.noop {
		w.logger.Info(fmt.Sprint(args...))
	}
}

func (w *wrapper) Warningf(format string, args ...interface{}) {
	if !w.noop {
		w.logger.Warning(fmt.Sprintf(format, args...))
	}
}

func (w *wrapper) Warning(args ...interface{}) {
	if !w.noop {
		w.logger.Warning(fmt.Sprint(args...))
	}
}

func (w *wrapper) Errorf(format string, args ...interface{}) {
	if !w.noop {
		w.logger.Error(nil, fmt.Sprintf(format, args...))
	}
}

func (w *wrapper) Error(args ...interface{}) {
	if !w.noop {
		w.logger.Error(nil, fmt.Sprint(args...))
	}
}

func (w *wrapper) WithValues(keysAndValues ...interface{}) Logger {
	if w.noop {
		return w
	}
	return &wrapper{
		trace:   w.trace.WithValues(keysAndValues...).Build(),
		logger:  w.logger.WithValues(keysAndValues...).Build(),
		metrics: w.metrics,
	}
}

func translateLevel(verbosity logrus.Level) log.Level {
	switch verbosity {
	case logrus.DebugLevel:
		return log.VerbosityDebug
	case logrus.InfoLevel:
		return log.VerbosityInfo
	case logrus.WarnLevel:
		return log.VerbosityWarning
	case logrus.ErrorLevel:
		return log.VerbosityError
	default:
		return log.VerbosityAll
	}
}

var noop = &wrapper{noop: true}

// Noop returns a logger which does not log.
func Noop() Logger {
	return noop
}

func New(w io.Writer, verbosity logrus.Level) Logger {
	metrics := newMetrics()
	log.ModifyDefaults(
		log.WithSink(w),
		log.WithTimestamp(),
		log.WithJSONOutput(),
		log.WithVerbosity(translateLevel(verbosity)),
		log.WithLevelHooks(log.VerbosityAll, metrics),
	)
	logger := log.NewLogger("legacy")
	return &wrapper{
		trace:   logger.V(1).Register(),
		logger:  logger,
		metrics: metrics,
	}
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package logging provides the logger interface abstraction
// and implementation for Bee. It uses logrus under the hood.
package logging

import (
	"io"

	"github.com/sirupsen/logrus"
)

type Logger interface {
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
	WithField(key string, value interface{}) *logrus.Entry
	WithFields(fields logrus.Fields) *logrus.Entry
	WriterLevel(logrus.Level) *io.PipeWriter
	NewEntry() *logrus.Entry
}

type logger struct {
	*logrus.Logger
	metrics metrics
}

func New(w io.Writer, level logrus.Level) Logger {
	l := logrus.New()
	l.SetOutput(w)
	l.SetLevel(level)
	l.Formatter = &logrus.TextFormatter{
		FullTimestamp: true,
	}
	metrics := newMetrics()
	l.AddHook(metrics)
	return &logger{
		Logger:  l,
		metrics: metrics,
	}
}

func (l *logger) NewEntry() *logrus.Entry {
	return logrus.NewEntry(l.Logger)
}

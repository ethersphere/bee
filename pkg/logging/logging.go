// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
	SetOutput(io.Writer)
}

func New(w io.Writer) Logger {
	l := logrus.New()
	l.SetOutput(w)
	l.Formatter = &logrus.TextFormatter{
		FullTimestamp: true,
	}
	return l
}

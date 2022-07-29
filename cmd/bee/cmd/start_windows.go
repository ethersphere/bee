// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package cmd

import (
	"fmt"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/sirupsen/logrus"
)

func isWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

func createWindowsEventLogger(svcName string, logger logging.Logger) (logging.Logger, error) {
	el, err := eventlog.Open(svcName)
	if err != nil {
		return nil, err
	}

	winlog := &windowsEventLogger{
		logger: logger,
		winlog: el,
	}

	return winlog, nil
}

type windowsEventLogger struct {
	logger logging.Logger
	winlog debug.Log
}

func (l *windowsEventLogger) Log(verbosity logrus.Level, args ...interface{}) {
	// ignore
}

func (l *windowsEventLogger) Tracef(format string, args ...interface{}) {
	// ignore
}

func (l *windowsEventLogger) Trace(args ...interface{}) {
	// ignore
}

func (l *windowsEventLogger) Debugf(format string, args ...interface{}) {
	// ignore
}

func (l *windowsEventLogger) Debug(args ...interface{}) {
	// ignore
}

func (l *windowsEventLogger) Infof(format string, args ...interface{}) {
	_ = l.winlog.Info(1633, fmt.Sprintf(format, args...))
}

func (l *windowsEventLogger) Info(args ...interface{}) {
	_ = l.winlog.Info(1633, fmt.Sprint(args...))
}

func (l *windowsEventLogger) Warningf(format string, args ...interface{}) {
	_ = l.winlog.Warning(1633, fmt.Sprintf(format, args...))
}

func (l *windowsEventLogger) Warning(args ...interface{}) {
	_ = l.winlog.Warning(1633, fmt.Sprint(args...))
}

func (l *windowsEventLogger) Errorf(format string, args ...interface{}) {
	_ = l.winlog.Error(1633, fmt.Sprintf(format, args...))
}

func (l *windowsEventLogger) Error(args ...interface{}) {
	_ = l.winlog.Error(1633, fmt.Sprint(args...))
}

func (l *windowsEventLogger) WithValues(keysAndValues ...interface{}) logging.Logger {
	return l.logger.WithValues(keysAndValues...)
}

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

	"github.com/ethersphere/bee/v2/pkg/log"
)

func isWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

func createWindowsEventLogger(svcName string, logger log.Logger) (log.Logger, error) {
	el, err := eventlog.Open(svcName)
	if err != nil {
		return nil, err
	}

	winlog := &windowsEventLogger{
		Logger: logger,
		winlog: el,
	}

	return winlog, nil
}

type windowsEventLogger struct {
	log.Logger
	winlog debug.Log
}

func (l windowsEventLogger) Debug(_ string, _ ...interface{}) {}

func (l windowsEventLogger) Info(msg string, keysAndValues ...interface{}) {
	_ = l.winlog.Info(1633, fmt.Sprintf("%s %s", msg, fmt.Sprintln(keysAndValues...)))
}

func (l windowsEventLogger) Warning(msg string, keysAndValues ...interface{}) {
	_ = l.winlog.Warning(1633, fmt.Sprintf("%s %s", msg, fmt.Sprintln(keysAndValues...)))
}

func (l windowsEventLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	if err != nil {
		keysAndValues = append(keysAndValues, "error", err)
	}
	_ = l.winlog.Error(1633, fmt.Sprintf("%s %s", msg, fmt.Sprintln(keysAndValues...)))
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows

package cmd

import (
	"errors"

	"github.com/ethersphere/bee/pkg/logging"
)

func isWindowsService() (bool, error) {
	return false, nil
}

func createWindowsEventLogger(svcName string, logger logging.Logger) (logging.Logger, error) {
	return nil, errors.New("cannot create Windows event logger")
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package cmd

import (
	"errors"

	"github.com/ethersphere/bee/v2/pkg/log"
)

func isWindowsService() (bool, error) {
	return false, nil
}

func createWindowsEventLogger(_ string, _ log.Logger) (log.Logger, error) {
	return nil, errors.New("cannot create Windows event logger")
}

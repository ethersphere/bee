// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build windows

package cmd

import (
	"golang.org/x/sys/windows/svc"
)

func (p *program) IsWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

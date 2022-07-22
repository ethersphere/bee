// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package libp2p

import "golang.org/x/sys/windows"

// Collection of errors returned by the underlying
// operating system that signals network unavailability.
var (
	errHostUnreachable    error = windows.WSAEHOSTUNREACH
	errNetworkUnreachable error = windows.WSAENETUNREACH
)

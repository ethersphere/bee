// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package kademlia

import "golang.org/x/sys/windows"

// Collection of errors returned by the libp2p library when the
// underlying operating system considers the network unreachable.
var (
	errHostUnreachable    error = windows.WSAEHOSTUNREACH
	errNetworkUnreachable error = windows.WSAENETUNREACH
)

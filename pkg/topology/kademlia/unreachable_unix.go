// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package kademlia

import "golang.org/x/sys/unix"

// Collection of errors returned by the libp2p library when the
// underlying operating system considers the network unreachable.
var (
	errHostUnreachable    error = unix.EHOSTUNREACH
	errNetworkUnreachable error = unix.ENETUNREACH
)

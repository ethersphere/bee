// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build js || wasip1

package libp2p

// Collection of errors returned by the underlying
// operating system that signals network unavailability.
var (
	errHostUnreachable    error = nil
	errNetworkUnreachable error = nil
)

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build js

package libp2p

import "errors"

// Collection of errors that signal network unavailability. In the browser
// there are no OS-level connect(2) errnos, so these are sentinel errors that
// will never match a real dial error (the unreachable-detection path is only
// exercised by the native TCP transport).
var (
	errHostUnreachable    = errors.New("host unreachable")
	errNetworkUnreachable = errors.New("network unreachable")
)

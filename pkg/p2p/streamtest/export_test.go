// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package streamtest

import "time"

func SetFullCloseTimeout(t time.Duration) {
	fullCloseTimeout = t
}

func ResetFullCloseTimeout() {
	fullCloseTimeout = fullCloseTimeoutDefault
}

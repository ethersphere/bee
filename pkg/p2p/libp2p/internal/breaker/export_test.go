// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breaker

import "time"

func SetTimeNow(f func() time.Time) {
	timeNow = f
}

func GetBackoffLimit() time.Duration {
	return backoffLimit
}

func GetFailInterval() time.Duration {
	return failInterval
}

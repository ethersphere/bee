// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breaker

func NewBreakerWithCurrentTimeFn(o Options, currentTimeFn currentTimeFn) Interface {
	return newBreakerWithCurrentTimeFn(o, currentTimeFn)
}

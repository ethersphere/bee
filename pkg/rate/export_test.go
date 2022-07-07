// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rate

func (r *Rate) SetTimeFunc(f func() int64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.now = f
}

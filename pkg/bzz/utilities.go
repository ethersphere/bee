// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz

// ContainsAddress reports whether a is present in addrs.
func ContainsAddress(addrs []Address, a *Address) bool {
	for _, v := range addrs {
		if v.Equal(a) {
			return true
		}
	}
	return false
}

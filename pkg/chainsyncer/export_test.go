// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsyncer

func SetNotifyHook(f func()) func() {
	var cleanup func()

	func(f func()) {
		cleanup = func() {
			notifyHook = f
		}
	}(notifyHook)
	notifyHook = f
	return cleanup
}

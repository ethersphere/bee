// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import ()

type mockDebouncer struct {
	set map[string]int64
}

func NewDebouncer() *mockDebouncer {
	a := &mockDebouncer{
		set: make(map[string]int64),
	}
	return a
}

func (md *mockDebouncer) Put(reference string, value int64) {
	md.set[reference] = value
}

func (md *mockDebouncer) Get(reference string) (int64, bool) {
	value, ok := md.set[reference]
	return value, ok
}

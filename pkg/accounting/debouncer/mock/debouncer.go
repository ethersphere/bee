// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import ()

type MockDebouncer struct {
	set map[string]int64
}

func NewDebouncer() *MockDebouncer {
	a := &MockDebouncer{
		set: make(map[string]int64),
	}
	return a
}

func (md *MockDebouncer) Put(reference string, value int64) {
	md.set[reference] = value
}

func (md *MockDebouncer) Get(reference string) (int64, bool) {
	value, ok := md.set[reference]
	return value, ok
}

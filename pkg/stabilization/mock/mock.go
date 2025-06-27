// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

type subscriber struct {
	stable bool
}

func NewSubscriber(stable bool) *subscriber {
	return &subscriber{
		stable: stable,
	}
}

func (s *subscriber) Subscribe() (<-chan struct{}, func()) {
	c := make(chan struct{})
	if s.stable {
		close(c)
	}
	return c, func() {}
}

func (s *subscriber) IsReady() bool {
	return s.stable
}

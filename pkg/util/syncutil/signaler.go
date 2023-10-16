// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncutil

import "sync"

// Signaler allows for multiple writers to safely signal an event
// so that reader on the channel C would get unblocked
type Signaler struct {
	C    chan struct{}
	once sync.Once
}

// NewSignaler initializes a new obj
func NewSignaler() *Signaler {
	return &Signaler{C: make(chan struct{})}
}

// Signal safely closes the blocking channel
func (s *Signaler) Signal() {
	s.once.Do(func() {
		close(s.C)
	})
}

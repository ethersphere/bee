// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flipflop

import (
	"time"
)

type detector struct {
	t         time.Duration
	worstCase time.Duration

	buf  chan struct{}
	out  chan struct{}
	quit chan struct{}
}

// NewFallingEdge returns a new falling edge detector.
// bufferTime is the time to buffer, worstCase is buffertime*worstcase time to wait before writing
// to the output anyway.
func NewFallingEdge(bufferTime, worstCase time.Duration) (in chan<- struct{}, out <-chan struct{}, clean func()) {
	d := &detector{
		t:         bufferTime,
		worstCase: worstCase,
		buf:       make(chan struct{}, 1),
		out:       make(chan struct{}),
		quit:      make(chan struct{}),
	}

	go d.work()

	return d.buf, d.out, func() { close(d.quit) }
}

func (d *detector) work() {
	var waitWrite <-chan time.Time
	var worstCase <-chan time.Time
	for {
		select {
		case <-d.quit:
			return
		case <-d.buf:
			// we have an item in the buffer, dont announce yet
			waitWrite = time.After(d.t)
			if worstCase == nil {
				worstCase = time.After(d.worstCase)
			}
		case <-waitWrite:
			select {
			case d.out <- struct{}{}:
			case <-d.quit:
				return
			}
			worstCase = nil
			waitWrite = nil
		case <-worstCase:
			select {
			case d.out <- struct{}{}:
			case <-d.quit:
				return
			}
			worstCase = nil
			waitWrite = nil
		}
	}
}

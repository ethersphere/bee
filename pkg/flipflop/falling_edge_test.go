// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flipflop_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/flipflop"
)

func TestFallingEdge(t *testing.T) {
	t.Skip("github actions")
	ok := make(chan struct{})
	tt := 50 * time.Millisecond
	worst := 5 * tt
	in, c, cleanup := flipflop.NewFallingEdge(tt, worst)
	defer cleanup()
	go func() {
		select {
		case <-c:
			close(ok)
			return
		case <-time.After(100 * time.Millisecond):
			t.Errorf("timed out")
		}
	}()

	in <- struct{}{}

	select {
	case <-ok:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out")
	}
}

func TestFallingEdgeBuffer(t *testing.T) {
	t.Skip("needs parameter tweaking on github actions")
	ok := make(chan struct{})
	tt := 150 * time.Millisecond
	worst := 9 * tt
	in, c, cleanup := flipflop.NewFallingEdge(tt, worst)
	defer cleanup()
	sleeps := 5
	wait := 50 * time.Millisecond

	start := time.Now()
	online := make(chan struct{})
	go func() {
		close(online)
		select {
		case <-c:
			if time.Since(start) <= 450*time.Millisecond {
				t.Errorf("wrote too early %v", time.Since(start))
			}
			close(ok)
			return
		case <-time.After(1000 * time.Millisecond):
			t.Errorf("timed out")
		}
	}()

	// wait for goroutine to be scheduled
	<-online

	for i := 0; i < sleeps; i++ {
		in <- struct{}{}
		time.Sleep(wait)
	}
	select {
	case <-ok:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out")
	}
}

func TestFallingEdgeWorstCase(t *testing.T) {
	t.Skip("github actions")
	ok := make(chan struct{})
	tt := 100 * time.Millisecond
	worst := 5 * tt
	in, c, cleanup := flipflop.NewFallingEdge(tt, worst)
	defer cleanup()
	sleeps := 9
	wait := 80 * time.Millisecond

	start := time.Now()

	go func() {
		select {
		case <-c:
			if time.Since(start) >= 550*time.Millisecond {
				t.Errorf("wrote too early %v", time.Since(start))
			}

			close(ok)
			return
		case <-time.After(1000 * time.Millisecond):
			t.Errorf("timed out")
		}
	}()
	go func() {
		for i := 0; i < sleeps; i++ {
			in <- struct{}{}
			time.Sleep(wait)
		}
	}()
	select {
	case <-ok:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out")
	}
}

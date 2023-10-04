// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/storageincentives"
)

func TestClose(t *testing.T) {
	t.Parallel()

	ev := storageincentives.NewEvents()

	done1 := make(chan struct{})
	done2 := make(chan struct{})
	done3 := make(chan struct{})

	ev.On(1, func(quit chan struct{}, blockNumber uint64) {
		<-quit
		close(done1)
	})

	ev.On(1, func(quit chan struct{}, blockNumber uint64) {
		<-quit
		close(done2)
	})

	ev.On(2, func(quit chan struct{}, blockNumber uint64) {
		<-quit
		close(done3)
	})

	ev.Publish(1, 0)
	ev.Publish(2, 0)

	ev.Close()

	for i, c := range []chan struct{}{done1, done2, done3} {
		select {
		case <-c:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for process %d to quit", i)
		}
	}
}

func TestPhaseCancel(t *testing.T) {
	t.Parallel()

	ev := storageincentives.NewEvents()

	done1 := make(chan struct{})
	done2 := make(chan struct{})
	done3 := make(chan struct{})
	defer ev.Close()

	// ensure no panics occur on an empty publish
	ev.Publish(0, 0)

	ev.On(1, func(quit chan struct{}, blockNumber uint64) {
		<-quit
		close(done1)
	})

	ev.On(2, func(quit chan struct{}, blockNumber uint64) {
		<-quit
		close(done2)
	})

	ev.On(3, func(quit chan struct{}, blockNumber uint64) {

	})

	ev.Publish(1, 0)
	ev.Publish(2, 0)
	ev.Publish(3, 0)
	ev.Cancel(1, 2)

	for i, c := range []chan struct{}{done1, done2} {
		select {
		case <-c:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for process %d to quit", i)
		}
	}
	select {
	case <-done3:
		t.Fatalf("process 3 quit unexpectedly")
	case <-time.After(50 * time.Millisecond):
	}
}

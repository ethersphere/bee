// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/storageincentives"
)

func TestClose(t *testing.T) {
	t.Parallel()

	ev := storageincentives.NewEvents()

	done1 := make(chan struct{})
	done2 := make(chan struct{})
	done3 := make(chan struct{})

	ev.On(1, func(ctx context.Context) {
		<-ctx.Done()
		close(done1)
	})

	ev.On(1, func(ctx context.Context) {
		<-ctx.Done()
		close(done2)
	})

	ev.On(2, func(ctx context.Context) {
		<-ctx.Done()
		close(done3)
	})

	ev.Publish(1)
	ev.Publish(2)

	ev.Close()

	for i := 0; i < 3; i++ {
		select {
		case <-done1:
		case <-done2:
		case <-done3:
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestPhaseCancel(t *testing.T) {
	t.Parallel()

	ev := storageincentives.NewEvents()

	done1 := make(chan struct{})
	done2 := make(chan struct{})
	defer ev.Close()

	// ensure no panics occur on an empty publish
	ev.Publish(0)

	ev.On(1, func(ctx context.Context) {
		<-ctx.Done()
		close(done1)
	})

	ev.On(2, func(ctx context.Context) {
		<-ctx.Done()
		close(done2)
	})

	ev.On(3, func(ctx context.Context) {
		ev.Cancel(1, 2)
	})

	ev.Publish(1)
	ev.Publish(2)
	ev.Publish(3)

	for i := 0; i < 2; i++ {
		select {
		case <-done1:
		case <-done2:
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/util"
)

func TestEarlyDone(t *testing.T) {
	wc := new(util.WaitingCounter)

	wc.Add(1)

	s := time.Now()

	go func() {
		time.Sleep(200 * time.Millisecond)
		wc.Done()
	}()

	r := wc.Wait(time.Second)

	if r != 0 {
		t.Fatal("counter not zero")
	}

	if time.Since(s) >= time.Second {
		t.Fatal("async done not detected")
	}
}

func TestDeadline(t *testing.T) {
	wc := new(util.WaitingCounter)

	wc.Add(1)

	s := time.Now()

	r := wc.Wait(200 * time.Millisecond)

	if r != 1 {
		t.Fatal("counter is zero, expected 1")
	}

	if time.Since(s) < 200*time.Millisecond {
		t.Fatal("expected to wait at least given duration")
	}
}

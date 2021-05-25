// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package metrics provides service for collecting various metrics about peers.
// It is intended to be used with the kademlia where the metrics are collected.
package waitnext_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology/kademlia/internal/waitnext"
)

func TestSet(t *testing.T) {

	waitNext := waitnext.New()

	addr := test.RandomAddress()

	waitNext.Set(addr, time.Now().Add(time.Millisecond*10), 2)

	if !waitNext.Waiting(addr) {
		t.Fatal("should be waiting")
	}

	time.Sleep(time.Millisecond * 11)

	if waitNext.Waiting(addr) {
		t.Fatal("should not be waiting")
	}

	if attempts := waitNext.Attempts(addr); attempts != 2 {
		t.Fatalf("want 2, got %d", attempts)
	}

	waitNext.SetTryAfter(addr, time.Now().Add(time.Millisecond*10))

	if !waitNext.Waiting(addr) {
		t.Fatal("should be waiting")
	}

	time.Sleep(time.Millisecond * 11)

	if waitNext.Waiting(addr) {
		t.Fatal("should not be waiting")
	}

	if attempts := waitNext.Attempts(addr); attempts != 2 {
		t.Fatalf("want 2, got %d", attempts)
	}
}

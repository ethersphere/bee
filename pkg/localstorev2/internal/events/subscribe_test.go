// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pullsync provides the pullsync protocol
// implementation.

package events_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/events"
)

func TestSubscriber(t *testing.T) {
	t.Parallel()

	s := events.NewSubscriber()

	bin, clean1 := s.Subscribe("0")
	t.Cleanup(func() { clean1() })
	go s.Trigger("0")
	<-bin

	select {
	case <-bin:
		t.Fatalf("trigger should not have fired")
	default:
	}

	bin1, clean2 := s.Subscribe("1")
	t.Cleanup(func() { clean2() })
	go s.Trigger("1")
	go s.Trigger("1")
	<-bin1
	<-bin1

	select {
	case <-bin:
		t.Fatalf("trigger should not have fired")
	default:
	}
}

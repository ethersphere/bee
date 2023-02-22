// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package events_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/events"
)

func TestSubscriber(t *testing.T) {
	t.Parallel()

	s := events.NewSubscriber()

	bin0_1, unsub0_1 := s.Subscribe("0")
	bin0_2, unsub0_2 := s.Subscribe("0")
	t.Cleanup(func() { unsub0_1(); unsub0_2() })
	go s.Trigger("0")
	<-bin0_1
	<-bin0_2

	select {
	case <-bin0_1:
		t.Fatalf("trigger should not have fired")
	case <-bin0_2:
		t.Fatalf("trigger should not have fired")
	default:
	}

	bin1, unsub1 := s.Subscribe("1")
	go s.Trigger("1")
	go s.Trigger("1")
	<-bin1
	<-bin1

	unsub1()

	select {
	case <-bin1:
		t.Fatalf("trigger should not have fired")
	default:
	}
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// eventually polls check briefly, tolerating that broker-side peer
// registration may not yet be visible the instant a client's Open call
// returns, since the two run in different goroutines.
func eventually(t *testing.T, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if check() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("condition not met before deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()

	broker, recorder, brokerAddr := newBroker(t, bps.Options{})
	spec := validSpec()

	client := bps.New(recorder, log.Noop, bps.Options{})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	})

	ss, err := client.Open(context.Background(), brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	eventually(t, func() bool {
		return len(broker.Status()) == 1
	})

	brokerStatus := broker.Status()
	if len(brokerStatus) != 1 {
		t.Fatalf("broker status: got %d entries want 1", len(brokerStatus))
	}
	bs := brokerStatus[0]
	if !bs.Broker {
		t.Fatal("broker status entry: got Broker=false want true")
	}
	if bs.Peers != 1 {
		t.Fatalf("broker status entry: got Peers=%d want 1", bs.Peers)
	}
	if !bs.Topic.Equal(swarm.NewAddress(spec.Topic)) {
		t.Fatalf("broker status entry: topic mismatch: got %x want %x", bs.Topic.Bytes(), spec.Topic)
	}
	if !bps.SpecEqual(bs.Spec, spec) {
		t.Fatal("broker status entry: spec mismatch")
	}

	clientStatus := client.Status()
	if len(clientStatus) != 1 {
		t.Fatalf("client status: got %d entries want 1", len(clientStatus))
	}
	cs := clientStatus[0]
	if cs.Broker {
		t.Fatal("client status entry: got Broker=true want false")
	}
	if cs.Peers != 1 {
		t.Fatalf("client status entry: got Peers=%d want 1", cs.Peers)
	}
	if !cs.Topic.Equal(swarm.NewAddress(spec.Topic)) {
		t.Fatalf("client status entry: topic mismatch: got %x want %x", cs.Topic.Bytes(), spec.Topic)
	}
	if !bps.SpecEqual(cs.Spec, spec) {
		t.Fatal("client status entry: spec mismatch")
	}
}

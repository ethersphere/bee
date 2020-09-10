// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

// TestSend creates a trojan chunk and sends it using push sync
func TestSend(t *testing.T) {
	var err error
	ctx := context.Background()

	// create a mock pushsync service to push the chunk to its destination
	var receipt *pushsync.Receipt
	var storedChunk swarm.Chunk
	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		rcpt := &pushsync.Receipt{
			Address: swarm.NewAddress(chunk.Address().Bytes()),
		}
		storedChunk = chunk
		receipt = rcpt
		return rcpt, nil
	})

	pss := pss.New(logging.New(ioutil.Discard, 0))
	pss.SetPushSyncer(pushSyncService)

	target := trojan.Target([]byte{1}) // arbitrary test target
	targets := trojan.Targets([]trojan.Target{target})
	payload := []byte("RECOVERY CHUNK")
	topic := trojan.NewTopic("RECOVERY TOPIC")

	// call Send to store trojan chunk in localstore
	if err = pss.Send(ctx, targets, topic, payload); err != nil {
		t.Fatal(err)
	}
	if receipt == nil {
		t.Fatal("no receipt")
	}

	m, err := trojan.Unwrap(storedChunk)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(m.Payload, payload) {
		t.Fatalf("payload mismatch expected %v but is %v instead", m.Payload, payload)
	}

	if !bytes.Equal(m.Topic[:], topic[:]) {
		t.Fatalf("topic mismatch expected %v but is %v instead", m.Topic, topic)
	}
}

// TestDeliver verifies that registering a handler on pss for a given topic and then submitting a trojan chunk with said topic to it
// results in the execution of the expected handler func
func TestDeliver(t *testing.T) {
	pss := pss.New(logging.New(ioutil.Discard, 0))
	ctx := context.Background()

	// test message
	topic := trojan.NewTopic("footopic")
	payload := []byte("foopayload")
	msg, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}
	// test chunk
	target := trojan.Target([]byte{1}) // arbitrary test target
	targets := trojan.Targets([]trojan.Target{target})
	c, err := msg.Wrap(ctx, targets)
	if err != nil {
		t.Fatal(err)
	}

	// create and register handler
	var tt trojan.Topic // test variable to check handler func was correctly called
	hndlr := func(ctx context.Context, m *trojan.Message) {
		tt = m.Topic // copy the message topic to the test variable
	}
	pss.Register(topic, hndlr)

	// call pss TryUnwrap on chunk and verify test topic variable value changes
	err = pss.TryUnwrap(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if tt != msg.Topic {
		t.Fatalf("unexpected result for pss Deliver func, expected test variable to have a value of %v but is %v instead", msg.Topic, tt)
	}
}

// TestRegister verifies that handler funcs are able to be registered correctly in pss
func TestRegister(t *testing.T) {
	var (
		pss     = pss.New(logging.New(ioutil.Discard, 0))
		h1Calls = 0
		h2Calls = 0
		h3Calls = 0
		mtx     sync.Mutex

		topic1  = trojan.NewTopic("one")
		topic2  = trojan.NewTopic("two")
		payload = []byte("payload")
		target  = trojan.Target([]byte{1})
		targets = trojan.Targets([]trojan.Target{target})

		h1 = func(_ context.Context, m *trojan.Message) {
			mtx.Lock()
			defer mtx.Unlock()
			h1Calls++
		}

		h2 = func(_ context.Context, m *trojan.Message) {
			mtx.Lock()
			defer mtx.Unlock()
			h2Calls++
		}

		h3 = func(_ context.Context, m *trojan.Message) {
			mtx.Lock()
			defer mtx.Unlock()
			h3Calls++
		}
	)

	_ = pss.Register(topic1, h1)
	_ = pss.Register(topic2, h2)

	// send a message on topic1, check that only h1 is called
	msg, err := trojan.NewMessage(topic1, payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err := msg.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}
	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	checkCalls(t, h1Calls, 1)
	checkCalls(t, h2Calls, 0)

	// register another topic handler on the same topic
	cleanup := pss.Register(topic1, h3)
	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	checkCalls(t, h1Calls, 2)
	checkCalls(t, h2Calls, 0)
	checkCalls(t, h3Calls, 1)

	cleanup() // remove the last handler

	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	checkCalls(t, h1Calls, 3)
	checkCalls(t, h2Calls, 0)
	checkCalls(t, h3Calls, 1)

	msg, err = trojan.NewMessage(topic2, payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err = msg.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}

	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	checkCalls(t, h1Calls, 3)
	checkCalls(t, h2Calls, 1)
	checkCalls(t, h3Calls, 1)
}

func checkCalls(t *testing.T, calls, exp int) {
	t.Helper()
	if calls != exp {
		t.Fatalf("expected %d calls on handler but got %d", exp, calls)
	}
}

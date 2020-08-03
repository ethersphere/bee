// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/trojan"
)

// TestTrojanChunkRetrieval creates a trojan chunk
// mocks the localstore
// calls pss.Send method and verifies it's properly stored
func TestTrojanChunkRetrieval(t *testing.T) {
	// TODO: REPLACE WITH FINISHED TEST
}

// TestPssMonitor creates a trojan chunk
// mocks the localstore
// calls pss.Send method
// updates the tag state (Stored/Sent/Synced)
// waits for the monitor to notify the changed state
func TestPssMonitor(t *testing.T) {
	var err error
	ctx := context.TODO()
	testTags := tags.NewTags()

	localStore := mock.NewTagsStorer(testTags)

	target := trojan.Target([]byte{1}) // arbitrary test target
	targets := trojan.Targets([]trojan.Target{target})
	payload := []byte("PSS CHUNK")
	topic := trojan.NewTopic("PSS TOPIC")

	var monitor *pss.Monitor

	pss := pss.NewPss(localStore, testTags)

	// call Send to store trojan chunk in localstore
	if monitor, err = pss.Send(ctx, targets, topic, payload); err != nil {
		t.Fatal(err)
	}

	storeTags := testTags.All()
	if len(storeTags) != 1 {
		t.Fatalf("expected %d tags got %d", 1, len(storeTags))
	}

	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(time.Second))
	defer cancel()

	for _, expectedState := range []tags.State{tags.StateStored, tags.StateSent, tags.StateSynced} {
		storeTags[0].Inc(expectedState)
	loop:
		for {
			// waits until the monitor state has changed or timeouts
			select {
			case state := <-monitor.State:
				if state == expectedState {
					break loop
				}
			case <-ctx.Done():
				t.Fatalf("no message received")
			}
		}
	}
}

// TestRegister verifies that handler funcs are able to be registered correctly in pss
func TestRegister(t *testing.T) {
	testTags := tags.NewTags()
	localStore := mock.NewTagsStorer(testTags)
	pss := pss.NewPss(localStore, testTags)

	// pss handlers should be empty
	if len(pss.GetAllHandlers()) != 0 {
		t.Fatalf("expected pss handlers to contain 0 elements, but its length is %d", len(pss.GetAllHandlers()))
	}

	handlerVerifier := 0 // test variable to check handler funcs are correctly retrieved

	// register first handler
	testHandler := func(m trojan.Message) {
		handlerVerifier = 1
	}
	testTopic := trojan.NewTopic("FIRST_HANDLER")
	pss.Register(testTopic, testHandler)

	if len(pss.GetAllHandlers()) != 1 {
		t.Fatalf("expected pss handlers to contain 1 element, but its length is %d", len(pss.GetAllHandlers()))
	}

	registeredHandler := pss.GetHandler(testTopic)
	registeredHandler(trojan.Message{}) // call handler to verify the retrieved func is correct

	if handlerVerifier != 1 {
		t.Fatalf("unexpected handler retrieved, verifier variable should be 1 but is %d instead", handlerVerifier)
	}

	// register second handler
	testHandler = func(m trojan.Message) {
		handlerVerifier = 2
	}
	testTopic = trojan.NewTopic("SECOND_HANDLER")
	pss.Register(testTopic, testHandler)
	if len(pss.GetAllHandlers()) != 2 {
		t.Fatalf("expected pss handlers to contain 2 elements, but its length is %d", len(pss.GetAllHandlers()))
	}

	registeredHandler = pss.GetHandler(testTopic)
	registeredHandler(trojan.Message{}) // call handler to verify the retrieved func is correct

	if handlerVerifier != 2 {
		t.Fatalf("unexpected handler retrieved, verifier variable should be 2 but is %d instead", handlerVerifier)
	}
}

// TestDeliver verifies that registering a handler on pss for a given topic and then submitting a trojan chunk with said topic to it
// results in the execution of the expected handler func
func TestDeliver(t *testing.T) {
	testTags := tags.NewTags()
	localStore := mock.NewTagsStorer(testTags)
	pss := pss.NewPss(localStore, testTags)

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
	c, err := msg.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}
	// trojan chunk has its type set through the validator called by the store, so this needs to be simulated
	c.WithType(swarm.ContentAddressed)

	// create and register handler
	var tt trojan.Topic // test variable to check handler func was correctly called
	hndlr := func(m trojan.Message) {
		tt = m.Topic // copy the message topic to the test variable
	}
	pss.Register(topic, hndlr)

	// call pss Deliver on chunk and verify test topic variable value changes
	pss.Deliver(c)
	if tt != msg.Topic {
		t.Fatalf("unexpected result for pss Deliver func, expected test variable to have a value of %v but is %v instead", msg.Topic, tt)
	}
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"bytes"
	"context"
	"io/ioutil"
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
	ctx := context.TODO()

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

	pss := pss.New(logging.New(ioutil.Discard, 0), pushSyncService)

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

// TestRegister verifies that handler funcs are able to be registered correctly in pss
func TestRegister(t *testing.T) {
	pss := pss.New(logging.New(ioutil.Discard, 0), nil)

	handlerVerifier := 0 // test variable to check handler funcs are correctly retrieved

	// register first handler
	testHandler := func(ctx context.Context, m *trojan.Message) error {
		handlerVerifier = 1
		return nil
	}
	testTopic := trojan.NewTopic("FIRST_HANDLER")
	pss.Register(testTopic, testHandler)

	registeredHandler := pss.GetHandler(testTopic)
	err := registeredHandler(context.Background(), &trojan.Message{}) // call handler to verify the retrieved func is correct
	if err != nil {
		t.Fatal(err)
	}

	if handlerVerifier != 1 {
		t.Fatalf("unexpected handler retrieved, verifier variable should be 1 but is %d instead", handlerVerifier)
	}

	// register second handler
	testHandler = func(ctx context.Context, m *trojan.Message) error {
		handlerVerifier = 2
		return nil
	}
	testTopic = trojan.NewTopic("SECOND_HANDLER")
	pss.Register(testTopic, testHandler)

	registeredHandler = pss.GetHandler(testTopic)
	err = registeredHandler(context.Background(), &trojan.Message{}) // call handler to verify the retrieved func is correct
	if err != nil {
		t.Fatal(err)
	}

	if handlerVerifier != 2 {
		t.Fatalf("unexpected handler retrieved, verifier variable should be 2 but is %d instead", handlerVerifier)
	}
}

// TestDeliver verifies that registering a handler on pss for a given topic and then submitting a trojan chunk with said topic to it
// results in the execution of the expected handler func
func TestDeliver(t *testing.T) {
	pss := pss.New(logging.New(ioutil.Discard, 0), nil)
	ctx := context.TODO()

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
	hndlr := func(ctx context.Context, m *trojan.Message) error {
		tt = m.Topic // copy the message topic to the test variable
		return nil
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

func TestHandler(t *testing.T) {
	pss := pss.New(logging.New(ioutil.Discard, 0), nil)
	testTopic := trojan.NewTopic("TEST_TOPIC")

	// verify handler is null
	if pss.GetHandler(testTopic) != nil {
		t.Errorf("handler should be null")
	}

	// register first handler
	testHandler := func(ctx context.Context, m *trojan.Message) error { return nil }

	// set handler for test topic
	pss.Register(testTopic, testHandler)

	if pss.GetHandler(testTopic) == nil {
		t.Errorf("handler should be registered")
	}

}

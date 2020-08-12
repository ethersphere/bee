// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"context"
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/trojan"
)

// TestTrojanChunkRetrieval creates a trojan chunk
// mocks the localstore
// calls pss.Send method and verifies it's properly stored
func TestTrojanChunkRetrieval(t *testing.T) {
	var err error
	ctx := context.TODO()
	testTags := tags.NewTags()

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

	pss := pss.New(logging.New(ioutil.Discard, 0), pushSyncService, testTags)

	target := trojan.Target([]byte{1}) // arbitrary test target
	targets := trojan.Targets([]trojan.Target{target})
	payload := []byte("RECOVERY CHUNK")
	topic := trojan.NewTopic("RECOVERY TOPIC")

	// call Send to store trojan chunk in localstore
	if _, err = pss.Send(ctx, targets, topic, payload); err != nil {
		t.Fatal(err)
	}

	// create a stored chunk artificially
	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}
	var tc swarm.Chunk
	tc, err = m.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}

	tag, err := tags.NewTags().Create("pss-chunks-tag", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	storedChunk = tc.WithTagID(tag.Uid)

	// check if receipt is received
	if receipt == nil {
		t.Fatal("receipt not received")
	}

	if !reflect.DeepEqual(tc, storedChunk) {
		t.Fatalf("trojan chunk created does not match sent chunk. got %s, want %s", storedChunk.Address().ByteString(), tc.Address().ByteString())
	}
}

// TestPssTags creates a trojan chunk
// mocks the localstore
// calls pss.Send method
// updates the tag state (Stored/Sent/Synced)
// waits for the monitor to notify the changed state
func TestPssTags(t *testing.T) {
	var err error
	ctx := context.TODO()
	testTags := tags.NewTags()

	target := trojan.Target([]byte{1}) // arbitrary test target
	targets := trojan.Targets([]trojan.Target{target})
	payload := []byte("PSS CHUNK")
	topic := trojan.NewTopic("PSS TOPIC")

	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		rcpt := &pushsync.Receipt{
			Address: swarm.NewAddress(chunk.Address().Bytes()),
		}
		tt, err := testTags.Get(chunk.TagID())
		if err != nil {
			t.Fatal(err)
		}
		tt.Inc(tags.StateSent)
		tt.Inc(tags.StateSynced)
		return rcpt, nil
	})

	pss := pss.New(logging.New(ioutil.Discard, 0), pushSyncService, testTags)

	var tag *tags.Tag
	// call Send to store trojan chunk in localstore
	if tag, err = pss.Send(ctx, targets, topic, payload); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1000 * time.Millisecond)

	storeTags := testTags.All()
	if len(storeTags) != 1 {
		t.Fatalf("expected %d tags got %d", 1, len(storeTags))
	}

	if tag.Get(tags.StateSent) != 1 && tag.Get(tags.StateSynced) != 1 {
		t.Fatalf("Trojan Chunk expected to be Sent == %d and Synced == %d", tag.Sent, tag.Synced)
	}

}

// TestRegister verifies that handler funcs are able to be registered correctly in pss
func TestRegister(t *testing.T) {
	testTags := tags.NewTags()
	pss := pss.New(logging.New(ioutil.Discard, 0), nil, testTags)

	handlerVerifier := 0 // test variable to check handler funcs are correctly retrieved

	// register first handler
	testHandler := func(m trojan.Message) {
		handlerVerifier = 1
	}
	testTopic := trojan.NewTopic("FIRST_HANDLER")
	pss.Register(testTopic, testHandler)

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
	pss := pss.New(logging.New(ioutil.Discard, 0), nil, testTags)
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

	testTags := tags.NewTags()
	pss := pss.New(logging.New(ioutil.Discard, 0), nil, testTags)
	testTopic := trojan.NewTopic("TEST_TOPIC")

	// verify handler is null
	if pss.GetHandler(testTopic) != nil {
		t.Errorf("handler should be null")
	}

	// register first handler
	testHandler := func(m trojan.Message) {}

	// set handler for test topic
	pss.Register(testTopic, testHandler)

	if pss.GetHandler(testTopic) == nil {
		t.Errorf("handler should be registered")
	}

}

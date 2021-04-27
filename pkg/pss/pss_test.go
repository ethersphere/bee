// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestSend creates a trojan chunk and sends it using push sync
func TestSend(t *testing.T) {
	var err error
	ctx := context.Background()

	// create a mock pushsync service to push the chunk to its destination
	var storedChunk swarm.Chunk
	pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
		storedChunk = chunk
		return nil, nil
	})
	p := pss.New(nil, logging.New(ioutil.Discard, 0))
	p.SetPushSyncer(pushSyncService)

	target := pss.Target([]byte{1}) // arbitrary test target
	targets := pss.Targets([]pss.Target{target})
	payload := []byte("some payload")
	topic := pss.NewTopic("topic")
	privkey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	recipient := &privkey.PublicKey
	s := &stamper{}
	// call Send to store trojan chunk in localstore
	if err = p.Send(ctx, topic, payload, s, recipient, targets); err != nil {
		t.Fatal(err)
	}

	topic1 := pss.NewTopic("topic-1")
	topic2 := pss.NewTopic("topic-2")
	topic3 := pss.NewTopic("topic-3")

	topics := []pss.Topic{topic, topic1, topic2, topic3}
	unwrapTopic, msg, err := pss.Unwrap(ctx, privkey, storedChunk, topics)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(msg, payload) {
		t.Fatalf("message mismatch: expected %x, got %x", payload, msg)
	}

	if !bytes.Equal(unwrapTopic[:], topic[:]) {
		t.Fatalf("topic mismatch: expected %x, got %x", topic[:], unwrapTopic[:])
	}
}

type topicMessage struct {
	topic pss.Topic
	msg   []byte
}

// TestDeliver verifies that registering a handler on pss for a given topic and then submitting a trojan chunk with said topic to it
// results in the execution of the expected handler func
func TestDeliver(t *testing.T) {
	privkey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	p := pss.New(privkey, logging.New(ioutil.Discard, 0))

	target := pss.Target([]byte{1}) // arbitrary test target
	targets := pss.Targets([]pss.Target{target})
	payload := []byte("some payload")
	topic := pss.NewTopic("topic")

	recipient := &privkey.PublicKey

	// test chunk
	chunk, err := pss.Wrap(context.Background(), topic, payload, recipient, targets)
	if err != nil {
		t.Fatal(err)
	}

	msgChan := make(chan topicMessage)

	// create and register handler
	handler := func(ctx context.Context, m []byte) {
		msgChan <- topicMessage{
			topic: topic,
			msg:   m,
		}
	}
	p.Register(topic, handler)

	// call pss TryUnwrap on chunk and verify test topic variable value changes
	p.TryUnwrap(chunk)

	var message topicMessage
	select {
	case message = <-msgChan:
		break
	case <-time.After(1 * time.Second):
		t.Fatal("reached timeout while waiting for message")
	}

	if !bytes.Equal(payload, message.msg) {
		t.Fatalf("message mismatch: expected %x, got %x", payload, message.msg)
	}

	if !bytes.Equal(topic[:], message.topic[:]) {
		t.Fatalf("topic mismatch: expected %x, got %x", topic[:], message.topic[:])
	}
}

// TestRegister verifies that handler funcs are able to be registered correctly in pss
func TestRegister(t *testing.T) {

	privkey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	recipient := &privkey.PublicKey
	var (
		p       = pss.New(privkey, logging.New(ioutil.Discard, 0))
		h1Calls = 0
		h2Calls = 0
		h3Calls = 0

		msgChan = make(chan struct{})

		topic1  = pss.NewTopic("one")
		topic2  = pss.NewTopic("two")
		payload = []byte("payload")
		target  = pss.Target([]byte{1})
		targets = pss.Targets([]pss.Target{target})

		h1 = func(_ context.Context, m []byte) {
			h1Calls++
			msgChan <- struct{}{}
		}

		h2 = func(_ context.Context, m []byte) {
			h2Calls++
			msgChan <- struct{}{}
		}

		h3 = func(_ context.Context, m []byte) {
			h3Calls++
			msgChan <- struct{}{}
		}
	)
	_ = p.Register(topic1, h1)
	_ = p.Register(topic2, h2)

	// send a message on topic1, check that only h1 is called
	chunk1, err := pss.Wrap(context.Background(), topic1, payload, recipient, targets)
	if err != nil {
		t.Fatal(err)
	}
	p.TryUnwrap(chunk1)

	waitHandlerCallback(t, &msgChan, 1)

	ensureCalls(t, &h1Calls, 1)
	ensureCalls(t, &h2Calls, 0)

	// register another topic handler on the same topic
	cleanup := p.Register(topic1, h3)
	p.TryUnwrap(chunk1)

	waitHandlerCallback(t, &msgChan, 2)

	ensureCalls(t, &h1Calls, 2)
	ensureCalls(t, &h2Calls, 0)
	ensureCalls(t, &h3Calls, 1)

	cleanup() // remove the last handler

	p.TryUnwrap(chunk1)

	waitHandlerCallback(t, &msgChan, 1)

	ensureCalls(t, &h1Calls, 3)
	ensureCalls(t, &h2Calls, 0)
	ensureCalls(t, &h3Calls, 1)

	chunk2, err := pss.Wrap(context.Background(), topic2, payload, recipient, targets)
	if err != nil {
		t.Fatal(err)
	}
	p.TryUnwrap(chunk2)

	waitHandlerCallback(t, &msgChan, 1)

	ensureCalls(t, &h1Calls, 3)
	ensureCalls(t, &h2Calls, 1)
	ensureCalls(t, &h3Calls, 1)
}

func waitHandlerCallback(t *testing.T, msgChan *chan struct{}, count int) {
	t.Helper()

	for received := 0; received < count; received++ {
		select {
		case <-*msgChan:
		case <-time.After(1 * time.Second):
			t.Fatal("reached timeout while waiting for handler message")
		}
	}
}

func ensureCalls(t *testing.T, calls *int, exp int) {
	t.Helper()

	if exp != *calls {
		t.Fatalf("expected %d calls, found %d", exp, *calls)
	}
}

type stamper struct{}

func (s *stamper) Stamp(_ swarm.Address) (*postage.Stamp, error) {
	return postagetesting.MustNewStamp(), nil
}

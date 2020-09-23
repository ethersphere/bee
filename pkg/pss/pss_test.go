// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
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
		rcpt := &pushsync.Receipt{
			Address: swarm.NewAddress(chunk.Address().Bytes()),
		}
		storedChunk = chunk
		receipt = rcpt
		return rcpt, nil
	})
	p := pss.New(logging.New(ioutil.Discard, 0))
	p.SetPushSyncer(pushSyncService)

	target := pss.Target([]byte{1}) // arbitrary test target
	targets := pss.Targets([]pss.Target{target})
	payload := []byte("RECOVERY CHUNK")
	topic := pss.NewTopic("RECOVERY TOPIC")
	privkey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	recipient := &privkey.PublicKey

	// call Send to store trojan chunk in localstore
	if err = p.Send(ctx, topic, payload, recipient, targets); err != nil {
		t.Fatal(err)
	}
	if receipt == nil {
		t.Fatal("no receipt")
	}

	topics := []pss.Topic{topic, topic1, topic2, topic3}
	topic, msg, err := pss.Unwrap(ctx, privkey, storedChunk, topics)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(msg, payload) {
		t.Fatalf("message mismatch: expected %x, got %x", payload, msg)
	}

	if !bytes.Equal(t[:], topic[:]) {
		t.Fatalf("topic mismatch: expected %x, got %x", topic[:], t[:])
	}
}

// TestDeliver verifies that registering a handler on pss for a given topic and then submitting a trojan chunk with said topic to it
// results in the execution of the expected handler func
func TestDeliver(t *testing.T) {
	pss := pss.New(logging.New(ioutil.Discard, 0))
	ctx := context.Background()
	var mtx sync.Mutex

	// test message
	topic := pss.NewTopic("footopic")
	payload := []byte("foopayload")
	msg, err := pss.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}
	// test chunk
	target := pss.Target([]byte{1}) // arbitrary test target
	targets := pss.Targets([]pss.Target{target})
	c, err := msg.Wrap(ctx, targets)
	if err != nil {
		t.Fatal(err)
	}

	// create and register handler
	var tt pss.Topic // test variable to check handler func was correctly called
	hndlr := func(ctx context.Context, m *pss.Message) {
		mtx.Lock()
		copy(tt[:], m.Topic[:]) // copy the message topic to the test variable
		mtx.Unlock()
	}
	pss.Register(topic, hndlr)

	// call pss TryUnwrap on chunk and verify test topic variable value changes
	err = pss.TryUnwrap(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	runtime.Gosched() // schedule the handler goroutine
	for i := 0; i < 10; i++ {
		mtx.Lock()

		eq := bytes.Equal(tt[:], msg.Topic[:])
		mtx.Unlock()
		if eq {
			return
		}
		<-time.After(50 * time.Millisecond)
	}
	t.Fatalf("unexpected result for pss Deliver func, expected test variable to have a value of %v but is %v instead", msg.Topic, tt)
}

// TestRegister verifies that handler funcs are able to be registered correctly in pss
func TestRegister(t *testing.T) {
	var (
		pss     = pss.New(logging.New(ioutil.Discard, 0))
		h1Calls = 0
		h2Calls = 0
		h3Calls = 0
		mtx     sync.Mutex

		topic1  = pss.NewTopic("one")
		topic2  = pss.NewTopic("two")
		payload = []byte("payload")
		target  = pss.Target([]byte{1})
		targets = pss.Targets([]pss.Target{target})

		h1 = func(_ context.Context, m *pss.Message) {
			mtx.Lock()
			defer mtx.Unlock()
			h1Calls++
		}

		h2 = func(_ context.Context, m *pss.Message) {
			mtx.Lock()
			defer mtx.Unlock()
			h2Calls++
		}

		h3 = func(_ context.Context, m *pss.Message) {
			mtx.Lock()
			defer mtx.Unlock()
			h3Calls++
		}
	)
	_ = pss.Register(topic1, h1)
	_ = pss.Register(topic2, h2)

	// send a message on topic1, check that only h1 is called
	msg, err := pss.NewMessage(topic1, payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err := msg.Wrap(context.Background(), targets)
	if err != nil {
		t.Fatal(err)
	}
	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	ensureCalls(t, &mtx, &h1Calls, 1)
	ensureCalls(t, &mtx, &h2Calls, 0)

	// register another topic handler on the same topic
	cleanup := pss.Register(topic1, h3)
	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	ensureCalls(t, &mtx, &h1Calls, 2)
	ensureCalls(t, &mtx, &h2Calls, 0)
	ensureCalls(t, &mtx, &h3Calls, 1)

	cleanup() // remove the last handler

	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	ensureCalls(t, &mtx, &h1Calls, 3)
	ensureCalls(t, &mtx, &h2Calls, 0)
	ensureCalls(t, &mtx, &h3Calls, 1)

	msg, err = pss.NewMessage(topic2, payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err = msg.Wrap(context.Background(), targets)
	if err != nil {
		t.Fatal(err)
	}

	err = pss.TryUnwrap(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	ensureCalls(t, &mtx, &h1Calls, 3)
	ensureCalls(t, &mtx, &h2Calls, 1)
	ensureCalls(t, &mtx, &h3Calls, 1)
}

func ensureCalls(t *testing.T, mtx *sync.Mutex, calls *int, exp int) {
	t.Helper()

	for i := 0; i < 10; i++ {
		mtx.Lock()
		if *calls == exp {
			mtx.Unlock()
			return
		}
		mtx.Unlock()
		<-time.After(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for value")
}

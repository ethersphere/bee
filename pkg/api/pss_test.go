// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	resolverMock "github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

func newTestWsServer(t *testing.T, o testServerOptions) *httptest.Server {

	if o.Logger == nil {
		o.Logger = logging.New(ioutil.Discard, 0)
	}
	if o.Resolver == nil {
		o.Resolver = resolverMock.NewResolver()
	}
	s := api.New(o.Tags, o.Storer, o.Resolver, o.Pss, o.Logger, nil, api.Options{
		GatewayMode: o.GatewayMode,
	})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)

	return ts
}

// creates a single websocket handler for an arbitrary topic, and receives a message
func TestPssWebsocketSingleHandler(t *testing.T) {
	// create a new pss instance, register a handle through ws, call
	// pss.TryUnwrap with a chunk designated for this handler and expect
	// the handler to be notified
	var (
		logger = logging.New(ioutil.Discard, 0)
		pss    = pss.New(logger)

		_, cl = newTestServer(t, testServerOptions{
			Pss:    pss,
			WsPath: "/pss/subscribe/testtopic",
			Storer: mock.NewStorer(),
			Logger: logger,
		})

		target     = trojan.Target([]byte{1})
		targets    = trojan.Targets([]trojan.Target{target})
		payload    = []byte("testdata")
		topic      = trojan.NewTopic("testtopic")
		msgContent = make([]byte, len(payload))
		tc         swarm.Chunk
		mtx        sync.Mutex
		timeout    = 5 * time.Second
		done       = make(chan struct{})
	)

	cl.SetReadDeadline(time.Now().Add(timeout))
	cl.SetReadLimit(swarm.ChunkSize)
	//cl.SetReadDeadline(time.Now().Add(pongWait))
	//cl.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}

			_, message, err := cl.ReadMessage()
			if err != nil {
				return
			}
			fmt.Println("got msg", message, msgContent)

			if message != nil {
				mtx.Lock()
				copy(msgContent, message)
				mtx.Unlock()
			}
		}
	}()
	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}

	tc, err = m.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}

	err = pss.TryUnwrap(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}
	waitMessage(t, msgContent, payload, timeout, &mtx)
}

func waitMessage(t *testing.T, data, expData []byte, timeout time.Duration, mtx *sync.Mutex) {
	ttl := time.After(timeout)
	for {
		select {
		case <-ttl:
			t.Fatal("timed out waiting for pss message")
		default:
		}
		mtx.Lock()
		if bytes.Equal(data, expData) {
			return
		}
		mtx.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

//func TestPssWebsocketSingleHandlerDeregister(t *testing.T) {

//}

//func TestPssWebsocketMultiHandler(t *testing.T) {

//}

// TestPssSend tests that the pss message sending over http works correctly.
func TestPssSend(t *testing.T) {
	var (
		logger = logging.New(os.Stdout, 5)

		mtx       sync.Mutex
		rxTargets trojan.Targets
		rxTopic   trojan.Topic
		rxBytes   []byte
		done      bool

		sendFn = func(_ context.Context, targets trojan.Targets, topic trojan.Topic, bytes []byte) error {
			mtx.Lock()
			rxTargets = targets
			rxTopic = topic
			rxBytes = bytes
			done = true
			mtx.Unlock()
			return nil
		}

		pss       = newMockPss(sendFn)
		client, _ = newTestServer(t, testServerOptions{
			Pss:    pss,
			Storer: mock.NewStorer(),
			Logger: logger,
		})

		targets   = fmt.Sprintf("[[%d]]", 0x12)
		payload   = []byte("testdata")
		topic     = "testtopic"
		hasher    = swarm.NewHasher()
		_, err    = hasher.Write([]byte(topic))
		topicHash = hasher.Sum(nil)

		//timeout = 5 * time.Second
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/testtopic/12", http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "OK",
				Code:    http.StatusOK,
			}),
		)
		waitDone(t, &mtx, &done)
		if !bytes.Equal(rxBytes, payload) {
			t.Fatalf("payload mismatch. want %v got %v", payload, rxBytes)
		}
		if targets != fmt.Sprint(rxTargets) {
			t.Fatalf("targets mismatch. want %v got %v", targets, rxTargets)
		}
		if string(topicHash) != string(rxTopic[:]) {
			t.Fatalf("topic mismatch. want %v got %v", topic, string(rxTopic[:]))
		}
	})
}

func waitDone(t *testing.T, mtx *sync.Mutex, done *bool) {
	for i := 0; i < 10; i++ {
		mtx.Lock()
		if *done {
			mtx.Unlock()
			return
		}
		mtx.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for send")
}

type pssSendFn func(context.Context, trojan.Targets, trojan.Topic, []byte) error
type mpss struct {
	f pssSendFn
}

func newMockPss(f pssSendFn) *mpss {
	return &mpss{f}
}

// Send arbitrary byte slice with the given topic to Targets.
func (m *mpss) Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, bytes []byte) error {
	return m.f(ctx, targets, topic, bytes)
}

// Register a Handler for a given Topic.
func (m *mpss) Register(_ trojan.Topic, _ pss.Handler) func() {
	panic("not implemented") // TODO: Implement
}

// TryUnwrap tries to unwrap a wrapped trojan message.
func (m *mpss) TryUnwrap(_ context.Context, _ swarm.Chunk) error {
	panic("not implemented") // TODO: Implement
}

func (m *mpss) SetPushSyncer(pushSyncer pushsync.PushSyncer) {
	panic("not implemented") // TODO: Implement
}

func (m *mpss) Close() error {
	panic("not implemented") // TODO: Implement
}

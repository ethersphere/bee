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
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
	"github.com/gorilla/websocket"
)

var (
	target  = trojan.Target([]byte{1})
	targets = trojan.Targets([]trojan.Target{target})
	payload = []byte("testdata")
	topic   = trojan.NewTopic("testtopic")
	timeout = 3 * time.Second
)

// creates a single websocket handler for an arbitrary topic, and receives a message
func TestPssWebsocketSingleHandler(t *testing.T) {
	var (
		pss, cl, _ = newPssTest(t, opts{})
		msgContent = make([]byte, len(payload))
		tc         swarm.Chunk
		mtx        sync.Mutex
		done       = make(chan struct{})
	)

	err := cl.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	defer close(done)
	go waitReadMessage(t, &mtx, cl, msgContent, done)
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
	waitMessage(t, msgContent, payload, &mtx)
}

func TestPssWebsocketSingleHandlerDeregister(t *testing.T) {
	// create a new pss instance, register a handle through ws, call
	// pss.TryUnwrap with a chunk designated for this handler and expect
	// the handler to be notified
	var (
		pss, cl, _ = newPssTest(t, opts{})
		msgContent = make([]byte, len(payload))
		tc         swarm.Chunk
		mtx        sync.Mutex
		done       = make(chan struct{})
	)

	err := cl.SetReadDeadline(time.Now().Add(timeout))

	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)
	defer close(done)
	go waitReadMessage(t, &mtx, cl, msgContent, done)
	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}

	tc, err = m.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}

	// close the websocket before calling pss with the message
	err = cl.WriteMessage(websocket.CloseMessage, []byte{})
	if err != nil {
		t.Fatal(err)
	}

	err = pss.TryUnwrap(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	waitMessage(t, msgContent, nil, &mtx)
}

func TestPssWebsocketMultiHandler(t *testing.T) {
	var (
		pss, cl, listener = newPssTest(t, opts{})
		u                 = url.URL{Scheme: "ws", Host: listener, Path: "/pss/subscribe/testtopic"}
		cl2, _, err       = websocket.DefaultDialer.Dial(u.String(), nil)

		msgContent  = make([]byte, len(payload))
		msgContent2 = make([]byte, len(payload))
		tc          swarm.Chunk
		mtx         sync.Mutex
		done        = make(chan struct{})
	)
	if err != nil {
		t.Fatalf("dial: %v. url %v", err, u.String())
	}

	err = cl.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	defer close(done)
	go waitReadMessage(t, &mtx, cl, msgContent, done)
	go waitReadMessage(t, &mtx, cl2, msgContent2, done)
	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}

	tc, err = m.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}

	// close the websocket before calling pss with the message
	err = cl.WriteMessage(websocket.CloseMessage, []byte{})
	if err != nil {
		t.Fatal(err)
	}

	err = pss.TryUnwrap(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	waitMessage(t, msgContent, nil, &mtx)
	waitMessage(t, msgContent2, nil, &mtx)
}

// TestPssSend tests that the pss message sending over http works correctly.
func TestPssSend(t *testing.T) {
	var (
		logger = logging.New(ioutil.Discard, 0)

		mtx             sync.Mutex
		recievedTargets trojan.Targets
		recievedTopic   trojan.Topic
		recievedBytes   []byte
		done            bool

		sendFn = func(_ context.Context, targets trojan.Targets, topic trojan.Topic, bytes []byte) error {
			mtx.Lock()
			recievedTargets = targets
			recievedTopic = topic
			recievedBytes = bytes
			done = true
			mtx.Unlock()
			return nil
		}

		pss          = newMockPss(sendFn)
		client, _, _ = newTestServer(t, testServerOptions{
			Pss:    pss,
			Storer: mock.NewStorer(),
			Logger: logger,
		})

		targets   = fmt.Sprintf("[[%d]]", 0x12)
		topic     = "testtopic"
		hasher    = swarm.NewHasher()
		_, err    = hasher.Write([]byte(topic))
		topicHash = hasher.Sum(nil)
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("err - bad targets", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/to/badtarget", http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Bad Request",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/testtopic/12", http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "OK",
				Code:    http.StatusOK,
			}),
		)
		waitDone(t, &mtx, &done)
		if !bytes.Equal(recievedBytes, payload) {
			t.Fatalf("payload mismatch. want %v got %v", payload, recievedBytes)
		}
		if targets != fmt.Sprint(recievedTargets) {
			t.Fatalf("targets mismatch. want %v got %v", targets, recievedTargets)
		}
		if string(topicHash) != string(recievedTopic[:]) {
			t.Fatalf("topic mismatch. want %v got %v", topic, string(recievedTopic[:]))
		}
	})
}

// TestPssPingPong tests that the websocket api adheres to the websocket standard
// and sends ping-pong messages to keep the connection alive.
// The test opens a websocket, keeps it alive for 500ms, then receives a pss message.
func TestPssPingPong(t *testing.T) {
	var (
		pss, cl, _ = newPssTest(t, opts{pingPeriod: 90 * time.Millisecond})

		msgContent = make([]byte, len(payload))
		tc         swarm.Chunk
		mtx        sync.Mutex
		pongWait   = 1 * time.Millisecond
		done       = make(chan struct{})
	)

	cl.SetReadLimit(swarm.ChunkSize)
	err := cl.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		t.Fatal(err)
	}
	defer close(done)
	go waitReadMessage(t, &mtx, cl, msgContent, done)

	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		t.Fatal(err)
	}

	tc, err = m.Wrap(targets)
	if err != nil {
		t.Fatal(err)
	}
	<-time.After(500 * time.Millisecond) // wait to see that the websocket is kept alive

	err = pss.TryUnwrap(context.Background(), tc)
	if err != nil {
		t.Fatal(err)
	}

	waitMessage(t, msgContent, nil, &mtx)
}

func waitReadMessage(t *testing.T, mtx *sync.Mutex, cl *websocket.Conn, targetContent []byte, done <-chan struct{}) {
	t.Helper()
	timeout := time.After(timeout)
	for {
		select {
		case <-done:
			return
		case <-timeout:
			t.Errorf("timed out waiting for message")
			return
		default:
		}

		msgType, message, err := cl.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.PongMessage {
			// ignore pings
			continue
		}

		if message != nil {
			mtx.Lock()
			copy(targetContent, message)
			mtx.Unlock()
		}
		<-time.After(50 * time.Millisecond)
	}
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

func waitMessage(t *testing.T, data, expData []byte, mtx *sync.Mutex) {
	ttl := time.After(timeout)
	for {
		select {
		case <-ttl:
			if expData == nil {
				return
			}
			t.Fatal("timed out waiting for pss message")
		default:
		}
		mtx.Lock()
		if bytes.Equal(data, expData) {
			mtx.Unlock()
			return
		}
		mtx.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

type opts struct {
	pingPeriod time.Duration
}

func newPssTest(t *testing.T, o opts) (pss.Interface, *websocket.Conn, string) {
	var (
		logger = logging.New(ioutil.Discard, 0)
		pss    = pss.New(logger)
	)
	if o.pingPeriod == 0 {
		o.pingPeriod = 10 * time.Second
	}
	_, cl, listener := newTestServer(t, testServerOptions{
		Pss:          pss,
		WsPath:       "/pss/subscribe/testtopic",
		Storer:       mock.NewStorer(),
		Logger:       logger,
		WsPingPeriod: o.pingPeriod,
	})
	return pss, cl, listener
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

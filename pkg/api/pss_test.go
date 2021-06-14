// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/websocket"
)

var (
	target  = pss.Target([]byte{1})
	targets = pss.Targets([]pss.Target{target})
	payload = []byte("testdata")
	topic   = pss.NewTopic("testtopic")
	// mTimeout is used to wait for checking the message contents, whereas rTimeout
	// is used to wait for reading the message. For the negative cases, i.e. ensuring
	// no message is received, the rTimeout might trigger before the mTimeout
	// (Issue #1388) causing test to fail. Hence the rTimeout should be slightly more
	// than the mTimeout
	mTimeout    = 10 * time.Second
	rTimeout    = 15 * time.Second
	longTimeout = 30 * time.Second
)

// creates a single websocket handler for an arbitrary topic, and receives a message
func TestPssWebsocketSingleHandler(t *testing.T) {
	var (
		p, publicKey, cl, _ = newPssTest(t, opts{})

		msgContent = make([]byte, len(payload))
		tc         swarm.Chunk
		mtx        sync.Mutex
		done       = make(chan struct{})
	)

	// the long timeout is needed so that we dont time out while still mining the message with Wrap()
	// otherwise the test (and other tests below) flakes
	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	defer close(done)
	go waitReadMessage(t, &mtx, cl, msgContent, done)

	tc, err = pss.Wrap(context.Background(), topic, payload, publicKey, targets)
	if err != nil {
		t.Fatal(err)
	}

	p.TryUnwrap(tc)

	waitMessage(t, msgContent, payload, &mtx)
}

func TestPssWebsocketSingleHandlerDeregister(t *testing.T) {
	// create a new pss instance, register a handle through ws, call
	// pss.TryUnwrap with a chunk designated for this handler and expect
	// the handler to be notified
	var (
		p, publicKey, cl, _ = newPssTest(t, opts{})

		msgContent = make([]byte, len(payload))
		tc         swarm.Chunk
		mtx        sync.Mutex
		done       = make(chan struct{})
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))

	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)
	defer close(done)
	go waitReadMessage(t, &mtx, cl, msgContent, done)

	tc, err = pss.Wrap(context.Background(), topic, payload, publicKey, targets)
	if err != nil {
		t.Fatal(err)
	}

	// close the websocket before calling pss with the message
	err = cl.WriteMessage(websocket.CloseMessage, []byte{})
	if err != nil {
		t.Fatal(err)
	}

	p.TryUnwrap(tc)

	waitMessage(t, msgContent, nil, &mtx)
}

func TestPssWebsocketMultiHandler(t *testing.T) {
	var (
		p, publicKey, cl, listener = newPssTest(t, opts{})

		u           = url.URL{Scheme: "ws", Host: listener, Path: "/pss/subscribe/testtopic"}
		cl2, _, err = websocket.DefaultDialer.Dial(u.String(), nil)

		msgContent  = make([]byte, len(payload))
		msgContent2 = make([]byte, len(payload))
		tc          swarm.Chunk
		mtx         sync.Mutex
		done        = make(chan struct{})
	)
	if err != nil {
		t.Fatalf("dial: %v. url %v", err, u.String())
	}

	err = cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	defer close(done)
	go waitReadMessage(t, &mtx, cl, msgContent, done)
	go waitReadMessage(t, &mtx, cl2, msgContent2, done)

	tc, err = pss.Wrap(context.Background(), topic, payload, publicKey, targets)
	if err != nil {
		t.Fatal(err)
	}

	// close the websocket before calling pss with the message
	err = cl.WriteMessage(websocket.CloseMessage, []byte{})
	if err != nil {
		t.Fatal(err)
	}

	p.TryUnwrap(tc)

	waitMessage(t, msgContent, nil, &mtx)
	waitMessage(t, msgContent2, nil, &mtx)
}

// TestPssSend tests that the pss message sending over http works correctly.
func TestPssSend(t *testing.T) {
	var (
		logger = logging.New(ioutil.Discard, 0)

		mtx             sync.Mutex
		receivedTopic   pss.Topic
		receivedBytes   []byte
		receivedTargets pss.Targets
		done            bool

		privk, _       = crypto.GenerateSecp256k1Key()
		publicKeyBytes = (*btcec.PublicKey)(&privk.PublicKey).SerializeCompressed()

		sendFn = func(ctx context.Context, targets pss.Targets, chunk swarm.Chunk) error {
			mtx.Lock()
			topic, msg, err := pss.Unwrap(ctx, privk, chunk, []pss.Topic{topic})
			receivedTopic = topic
			receivedBytes = msg
			receivedTargets = targets
			done = true
			mtx.Unlock()
			return err
		}
		mp           = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, 11, 10, 1000)))
		p            = newMockPss(sendFn)
		client, _, _ = newTestServer(t, testServerOptions{
			Pss:    p,
			Storer: mock.NewStorer(),
			Logger: logger,
			Post:   mp,
		})

		recipient = hex.EncodeToString(publicKeyBytes)
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
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/to/badtarget?recipient="+recipient, http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Bad Request",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("err - bad batch", func(t *testing.T) {
		hexbatch := hex.EncodeToString(batchInvalid)
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/to/12", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid postage batch id",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("ok batch", func(t *testing.T) {
		hexbatch := hex.EncodeToString(batchOk)
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/to/12", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
		)
	})
	t.Run("bad request - batch empty", func(t *testing.T) {
		hexbatch := hex.EncodeToString(batchEmpty)
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/to/12", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
		)
	})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/testtopic/12?recipient="+recipient, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Created",
				Code:    http.StatusCreated,
			}),
		)
		waitDone(t, &mtx, &done)
		if !bytes.Equal(receivedBytes, payload) {
			t.Fatalf("payload mismatch. want %v got %v", payload, receivedBytes)
		}
		if targets != fmt.Sprint(receivedTargets) {
			t.Fatalf("targets mismatch. want %v got %v", targets, receivedTargets)
		}
		if string(topicHash) != string(receivedTopic[:]) {
			t.Fatalf("topic mismatch. want %v got %v", topic, string(receivedTopic[:]))
		}
	})

	t.Run("without recipient", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/testtopic/12", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Created",
				Code:    http.StatusCreated,
			}),
		)
		waitDone(t, &mtx, &done)
		if !bytes.Equal(receivedBytes, payload) {
			t.Fatalf("payload mismatch. want %v got %v", payload, receivedBytes)
		}
		if targets != fmt.Sprint(receivedTargets) {
			t.Fatalf("targets mismatch. want %v got %v", targets, receivedTargets)
		}
		if string(topicHash) != string(receivedTopic[:]) {
			t.Fatalf("topic mismatch. want %v got %v", topic, string(receivedTopic[:]))
		}
	})
}

// TestPssPingPong tests that the websocket api adheres to the websocket standard
// and sends ping-pong messages to keep the connection alive.
// The test opens a websocket, keeps it alive for 500ms, then receives a pss message.
func TestPssPingPong(t *testing.T) {
	var (
		p, publicKey, cl, _ = newPssTest(t, opts{pingPeriod: 90 * time.Millisecond})

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

	tc, err = pss.Wrap(context.Background(), topic, payload, publicKey, targets)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond) // wait to see that the websocket is kept alive

	p.TryUnwrap(tc)

	waitMessage(t, msgContent, nil, &mtx)
}

func waitReadMessage(t *testing.T, mtx *sync.Mutex, cl *websocket.Conn, targetContent []byte, done <-chan struct{}) {
	t.Helper()
	timeout := time.After(rTimeout)
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
		time.Sleep(50 * time.Millisecond)
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
	t.Helper()

	ttl := time.After(mTimeout)
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
		time.Sleep(100 * time.Millisecond)
	}
}

type opts struct {
	pingPeriod time.Duration
}

func newPssTest(t *testing.T, o opts) (pss.Interface, *ecdsa.PublicKey, *websocket.Conn, string) {
	t.Helper()

	privkey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	var (
		logger = logging.New(ioutil.Discard, 0)
		pss    = pss.New(privkey, logger)
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
	return pss, &privkey.PublicKey, cl, listener
}

type pssSendFn func(context.Context, pss.Targets, swarm.Chunk) error
type mpss struct {
	f pssSendFn
}

func newMockPss(f pssSendFn) *mpss {
	return &mpss{f}
}

// Send arbitrary byte slice with the given topic to Targets.
func (m *mpss) Send(ctx context.Context, topic pss.Topic, payload []byte, _ postage.Stamper, recipient *ecdsa.PublicKey, targets pss.Targets) error {
	chunk, err := pss.Wrap(ctx, topic, payload, recipient, targets)
	if err != nil {
		return err
	}
	return m.f(ctx, targets, chunk)
}

// Register a Handler for a given Topic.
func (m *mpss) Register(_ pss.Topic, _ pss.Handler) func() {
	panic("not implemented") // TODO: Implement
}

// TryUnwrap tries to unwrap a wrapped trojan message.
func (m *mpss) TryUnwrap(_ swarm.Chunk) {
	panic("not implemented") // TODO: Implement
}

func (m *mpss) SetPushSyncer(pushSyncer pushsync.PushSyncer) {
	panic("not implemented") // TODO: Implement
}

func (m *mpss) Close() error {
	panic("not implemented") // TODO: Implement
}

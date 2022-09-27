// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
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
	"github.com/ethersphere/bee/pkg/log"
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
	mTimeout    = 2 * time.Second
	rTimeout    = 2 * mTimeout
	longTimeout = 30 * time.Second
)

// creates a single websocket handler for an arbitrary topic, and receives a message
func TestPssWebsocketSingleHandler(t *testing.T) {
	t.Parallel()

	var (
		p, publicKey, cl, _ = newPssTest(t, opts{})
		respC               = make(chan error, 1)
		msgC                = make(chan []byte)
		tc                  swarm.Chunk
	)

	// the long timeout is needed so that we dont time out while still mining the message with Wrap()
	// otherwise the test (and other tests below) flakes
	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	tc, err = pss.Wrap(context.Background(), topic, payload, publicKey, targets)
	if err != nil {
		t.Fatal(err)
	}

	p.TryUnwrap(tc)

	go readMessage(t, cl, msgC)
	go expectMessage(t, respC, msgC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

func TestPssWebsocketSingleHandlerDeregister(t *testing.T) {
	t.Parallel()

	// create a new pss instance, register a handle through ws, call
	// pss.TryUnwrap with a chunk designated for this handler and expect
	// the handler to be notified
	var (
		p, publicKey, cl, _ = newPssTest(t, opts{})
		respC               = make(chan error, 1)
		msgC                = make(chan []byte)
		tc                  swarm.Chunk
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))

	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

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

	go readMessage(t, cl, msgC)
	go expectMessage(t, respC, msgC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

func TestPssWebsocketMultiHandler(t *testing.T) {
	t.Parallel()

	var (
		p, publicKey, cl, listener = newPssTest(t, opts{})

		u           = url.URL{Scheme: "ws", Host: listener, Path: "/pss/subscribe/testtopic"}
		cl2, _, err = websocket.DefaultDialer.Dial(u.String(), nil)

		respC = make(chan error, 2)
		msg2C = make(chan []byte)
		msgC  = make(chan []byte)
		tc    swarm.Chunk
	)
	if err != nil {
		t.Fatalf("dial: %v. url %v", err, u.String())
	}

	err = cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

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

	go readMessage(t, cl, msgC)
	go readMessage(t, cl2, msg2C)
	go expectMessage(t, respC, msg2C, payload)
	go expectMessage(t, respC, msgC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

// nolint:paralleltest
// TestPssSend tests that the pss message sending over http works correctly.
func TestPssSend(t *testing.T) {
	var (
		logger = log.Noop

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
		mp              = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)))
		p               = newMockPss(sendFn)
		client, _, _, _ = newTestServer(t, testServerOptions{
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
				Message: "target is not valid hex string",
				Code:    http.StatusBadRequest,
			}),
		)

		// If this test needs to be modified (most probably because the max target length changed)
		// the please verify that SwarmCommon.yaml -> components -> PssTarget also reflects the correct value
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/to/123456789abcdf?recipient="+recipient, http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "hex string target exceeds max length of 6",
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
	t.Parallel()

	var (
		p, publicKey, cl, _ = newPssTest(t, opts{pingPeriod: 90 * time.Millisecond})

		respC    = make(chan error, 1)
		msgC     = make(chan []byte)
		tc       swarm.Chunk
		pongWait = 1 * time.Millisecond
		done     = make(chan struct{})
	)

	cl.SetReadLimit(swarm.ChunkSize)
	err := cl.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		t.Fatal(err)
	}
	defer close(done)

	tc, err = pss.Wrap(context.Background(), topic, payload, publicKey, targets)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond) // wait to see that the websocket is kept alive

	p.TryUnwrap(tc)

	go readMessage(t, cl, msgC)
	go expectMessage(t, respC, msgC, nil)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

func readMessage(t *testing.T, cl *websocket.Conn, msgC chan []byte) {
	t.Helper()

	timeout := time.NewTimer(rTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			t.Error("timed out waiting for message")
			return
		default:
			msgType, message, err := cl.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.PongMessage {
				// ignore pings
				continue
			}
			if message == nil {
				continue
			}

			msgC <- message

			return
		}
	}
}

func waitDone(t *testing.T, mtx *sync.Mutex, done *bool) {
	t.Helper()

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

func expectMessage(t *testing.T, respC chan error, msgC chan []byte, expData []byte) {
	t.Helper()

	timeout := time.NewTimer(mTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			if expData == nil {
				respC <- nil
				return
			}
			respC <- errors.New("timed out waiting for pss message")
			return
		case msg := <-msgC:
			if bytes.Equal(msg, expData) {
				respC <- nil
				return
			}
			respC <- errors.New("unexpected message")
			return
		}
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
		logger = log.Noop
		pss    = pss.New(privkey, logger)
	)
	if o.pingPeriod == 0 {
		o.pingPeriod = 10 * time.Second
	}
	_, cl, listener, _ := newTestServer(t, testServerOptions{
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

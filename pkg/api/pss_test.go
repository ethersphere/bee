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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/gorilla/websocket"
)

var (
	target      = pss.Target([]byte{1})
	targets     = pss.Targets([]pss.Target{target})
	payload     = []byte("testdata")
	topic       = pss.NewTopic("testtopic")
	mTimeout    = 2 * time.Second
	longTimeout = 30 * time.Second
)

// creates a single websocket handler for an arbitrary topic, and receives a message
func TestPssWebsocketSingleHandler(t *testing.T) {
	t.Parallel()

	var (
		p, publicKey, cl, _ = newPssTest(t, opts{})
		respC               = make(chan error, 1)
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

	go expectMessage(t, cl, respC, payload)
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

	go expectMessage(t, cl, respC, payload)
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
		tc    swarm.Chunk
	)
	if err != nil {
		t.Fatalf("dial: %v. url %v", err, u.String())
	}
	testutil.CleanupCloser(t, cl2)

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

	go expectMessage(t, cl, respC, payload)
	go expectMessage(t, cl2, respC, payload)
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
		mtx             sync.Mutex
		receivedTopic   pss.Topic
		receivedBytes   []byte
		receivedTargets pss.Targets
		done            bool

		privk, _       = crypto.GenerateSecp256k1Key()
		publicKeyBytes = crypto.EncodeSecp256k1PublicKey(&privk.PublicKey)

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
			Storer: mockstorer.New(),
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

	t.Run("err - bad batch", func(t *testing.T) {
		hexbatch := "abcdefgg"
		jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/to/12", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
			jsonhttptest.WithRequestBody(bytes.NewReader(payload)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid header params",
				Reasons: []jsonhttp.Reason{
					{
						Field: api.SwarmPostageBatchIdHeader,
						Error: api.HexInvalidByteError('g').Error(),
					},
				},
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
		tc       swarm.Chunk
		pongWait = 1 * time.Millisecond
	)

	cl.SetReadLimit(swarm.ChunkSize)
	err := cl.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		t.Fatal(err)
	}

	tc, err = pss.Wrap(context.Background(), topic, payload, publicKey, targets)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond) // wait to see that the websocket is kept alive

	p.TryUnwrap(tc)

	go expectMessage(t, cl, respC, nil)
	if err := <-respC; err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		// note: error has *websocket.netError type so we need to check error by checking message
		t.Fatal("want timeout error")
	}
}

func expectMessage(t *testing.T, cl *websocket.Conn, respC chan error, expData []byte) {
	t.Helper()

	timeout := time.NewTimer(mTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			if expData == nil {
				respC <- nil
			} else {
				respC <- errors.New("timed out waiting for message")
			}
			return
		default:
			msgType, message, err := cl.ReadMessage()
			if err != nil {
				respC <- err
				return
			}
			if msgType == websocket.PongMessage {
				// ignore pings
				continue
			}
			if message == nil {
				continue
			}

			if bytes.Equal(message, expData) {
				respC <- nil
			} else {
				respC <- errors.New("unexpected message")
			}
			return
		}
	}
}

func waitDone(t *testing.T, mtx *sync.Mutex, done *bool) {
	t.Helper()

	err := spinlock.Wait(time.Second, func() bool {
		mtx.Lock()
		defer mtx.Unlock()
		return *done
	})
	if err != nil {
		t.Fatal("timed out waiting for send")
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

	pss := pss.New(privkey, log.Noop)
	testutil.CleanupCloser(t, pss)

	if o.pingPeriod == 0 {
		o.pingPeriod = 10 * time.Second
	}
	_, cl, listener, _ := newTestServer(t, testServerOptions{
		Pss:          pss,
		WsPath:       "/pss/subscribe/testtopic",
		Storer:       mockstorer.New(),
		Logger:       log.Noop,
		WsPingPeriod: o.pingPeriod,
	})

	return pss, &privkey.PublicKey, cl, listener
}

func TestPssPostHandlerInvalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		topic   string
		targets string
		want    jsonhttp.StatusResponse
	}{{
		name:    "targets - odd length hex string",
		topic:   "test_topic",
		targets: "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "target",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "targets - odd length hex string",
		topic:   "test_topic",
		targets: "1G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "target",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodPost, "/pss/send/"+tc.topic+"/"+tc.targets, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

type (
	pssSendFn func(context.Context, pss.Targets, swarm.Chunk) error
	mpss      struct {
		f pssSendFn
	}
)

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

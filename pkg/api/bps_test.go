// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/gorilla/websocket"
)

type fakeBPS struct {
	sess    *fakeSession
	joinErr error

	mu     sync.Mutex
	broker swarm.Address
	addr   swarm.Address
}

func (f *fakeBPS) Join(_ context.Context, broker, addr swarm.Address) (api.BPSSession, error) {
	f.mu.Lock()
	f.broker, f.addr = broker, addr
	f.mu.Unlock()
	if f.joinErr != nil {
		return nil, f.joinErr
	}
	return f.sess, nil
}

type fakeSession struct {
	challenge []byte
	msgs      chan []byte
	done      chan struct{}
	claims    chan []byte
	published chan []byte
	closed    chan struct{}

	claimErr   error
	publishErr error

	doneOnce  sync.Once
	closeOnce sync.Once
	err       error
}

func newFakeSession() *fakeSession {
	challenge := make([]byte, 32)
	copy(challenge, "bps-test-challenge")
	return &fakeSession{
		challenge: challenge,
		msgs:      make(chan []byte),
		done:      make(chan struct{}),
		claims:    make(chan []byte, 1),
		published: make(chan []byte, 8),
		closed:    make(chan struct{}),
	}
}

func (f *fakeSession) Challenge() []byte       { return f.challenge }
func (f *fakeSession) Messages() <-chan []byte { return f.msgs }
func (f *fakeSession) Done() <-chan struct{}   { return f.done }
func (f *fakeSession) Err() error              { return f.err }

func (f *fakeSession) Claim(_ context.Context, b []byte) error {
	if f.claimErr != nil {
		return f.claimErr
	}
	f.claims <- b
	return nil
}

func (f *fakeSession) Publish(_ context.Context, b []byte) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published <- b
	return nil
}

func (f *fakeSession) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

// end simulates the broker stream ending with err.
func (f *fakeSession) end(err error) {
	f.doneOnce.Do(func() {
		f.err = err
		close(f.done)
	})
}

type bpsFixture struct {
	client   *http.Client
	listener string
	bps      *fakeBPS
	sess     *fakeSession
	signer   crypto.Signer
	owner    []byte
	topic    []byte
	id       []byte
	addr     swarm.Address
	broker   swarm.Address
	self     swarm.Address
}

func newBPSFixture(t *testing.T) *bpsFixture {
	t.Helper()
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	topic := make([]byte, 32)
	copy(topic, "bps-test-topic")
	id, err := crypto.LegacyKeccak256(append([]byte("bps-claim"), topic...))
	if err != nil {
		t.Fatal(err)
	}
	addr, err := soc.CreateAddress(id, owner.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	sess := newFakeSession()
	f := &bpsFixture{
		bps:    &fakeBPS{sess: sess},
		sess:   sess,
		signer: signer,
		owner:  owner.Bytes(),
		topic:  topic,
		id:     id,
		addr:   addr,
		broker: swarm.RandAddress(t),
		self:   swarm.RandAddress(t),
	}
	f.client, _, f.listener, _, _ = newTestServer(t, testServerOptions{
		Bps:          f.bps,
		Overlay:      f.self,
		WsPingPeriod: 60 * time.Second,
	})
	return f
}

func (f *bpsFixture) path(endpoint string) string {
	return fmt.Sprintf("/bps/%s/%s/%s", endpoint, hex.EncodeToString(f.owner), hex.EncodeToString(f.topic))
}

func (f *bpsFixture) dial(t *testing.T, endpoint string) *websocket.Conn {
	t.Helper()
	u := url.URL{Scheme: "ws", Host: f.listener, Path: f.path(endpoint), RawQuery: "broker=" + f.broker.String()}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial %s: %v", u.String(), err)
	}
	testutil.CleanupCloser(t, conn)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return conn
}

// signedSOC returns full SOC bytes at f.addr wrapping payload.
func (f *bpsFixture) signedSOC(t *testing.T, signer crypto.Signer, payload []byte) []byte {
	t.Helper()
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err := soc.New(f.id, ch).Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	return c.Data()
}

// claimSignature signs the claim payload challenge|broker with signer.
func (f *bpsFixture) claimSignature(t *testing.T, signer crypto.Signer, challenge []byte) string {
	t.Helper()
	payload := append(append([]byte{}, challenge...), f.broker.Bytes()...)
	data := f.signedSOC(t, signer, payload)
	return hex.EncodeToString(data[swarm.HashSize : swarm.HashSize+swarm.SocSignatureSize])
}

func expectClose(t *testing.T, conn *websocket.Conn, code int) {
	t.Helper()
	for {
		_, _, err := conn.ReadMessage()
		if err == nil {
			continue
		}
		var ce *websocket.CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("want close error with code %d, got %v", code, err)
		}
		if ce.Code != code {
			t.Fatalf("want close code %d, got %d (%q)", code, ce.Code, ce.Text)
		}
		return
	}
}

func TestBPSPreUpgrade(t *testing.T) {
	t.Parallel()

	f := newBPSFixture(t)
	nilClient, _, _, _, _ := newTestServer(t, testServerOptions{})
	owner := hex.EncodeToString(f.owner)
	topic := hex.EncodeToString(f.topic)

	for _, endpoint := range []string{"subscribe", "publish"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			base := fmt.Sprintf("/bps/%s/%s/%s", endpoint, owner, topic)

			jsonhttptest.Request(t, f.client, http.MethodGet, base, http.StatusBadRequest,
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Code: http.StatusBadRequest, Message: "missing broker",
				}),
			)
			jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker=zz", http.StatusBadRequest)
			jsonhttptest.Request(t, f.client, http.MethodGet,
				fmt.Sprintf("/bps/%s/%s/%s?broker=%s", endpoint, "zz", topic, f.broker), http.StatusBadRequest)
			jsonhttptest.Request(t, f.client, http.MethodGet,
				fmt.Sprintf("/bps/%s/%s/%s?broker=%s", endpoint, owner, "abcd", f.broker), http.StatusBadRequest)
			jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker=abcd", http.StatusBadRequest,
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Code: http.StatusBadRequest, Message: "invalid broker",
				}),
			)
			jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker="+f.self.String(), http.StatusBadRequest,
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Code: http.StatusBadRequest, Message: "broker cannot be this node",
				}),
			)
			jsonhttptest.Request(t, nilClient, http.MethodGet, base+"?broker="+f.broker.String(), http.StatusServiceUnavailable)
		})
	}
}

func TestBPSSubscribe(t *testing.T) {
	t.Parallel()

	t.Run("forwards valid soc", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t, "subscribe")
		msg := f.signedSOC(t, f.signer, []byte("hello cohort"))
		f.sess.msgs <- msg
		typ, got, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if typ != websocket.BinaryMessage || !bytes.Equal(got, msg) {
			t.Fatalf("got type %d %x, want binary %x", typ, got, msg)
		}
		f.bps.mu.Lock()
		gotAddr, gotBroker := f.bps.addr, f.bps.broker
		f.bps.mu.Unlock()
		if !gotAddr.Equal(f.addr) || !gotBroker.Equal(f.broker) {
			t.Fatalf("join got addr %s broker %s, want %s %s", gotAddr, gotBroker, f.addr, f.broker)
		}
	})

	t.Run("drops invalid soc", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t, "subscribe")
		other, _ := crypto.GenerateSecp256k1Key()
		f.sess.msgs <- f.signedSOC(t, crypto.NewDefaultSigner(other), []byte("forged"))
		f.sess.msgs <- []byte("garbage")
		valid := f.signedSOC(t, f.signer, []byte("real"))
		f.sess.msgs <- valid
		_, got, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, valid) {
			t.Fatalf("got %x, want only the valid soc %x", got, valid)
		}
	})

	t.Run("client frame closes 4002", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t, "subscribe")
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte("nope")); err != nil {
			t.Fatal(err)
		}
		expectClose(t, conn, api.BPSCloseInvalidMessage)
		<-f.sess.closed
	})

	t.Run("broker gone closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t, "subscribe")
		f.sess.end(errors.New("stream reset"))
		expectClose(t, conn, api.BPSCloseBrokerGone)
		<-f.sess.closed
	})

	t.Run("join error closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		f.bps.joinErr = errors.New("broker unreachable")
		conn := f.dial(t, "subscribe")
		expectClose(t, conn, api.BPSCloseBrokerGone)
	})
}

func TestBPSSubscribeMessagesClosed(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dial(t, "subscribe")
	close(f.sess.msgs)
	expectClose(t, conn, api.BPSCloseBrokerGone)
	<-f.sess.closed
}

// waitSessionClosed fails the test if the session is not closed in time.
func waitSessionClosed(t *testing.T, f *bpsFixture) {
	t.Helper()
	select {
	case <-f.sess.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("session not closed")
	}
}

// readChallenge reads the challenge frame and checks its fields.
func readChallenge(t *testing.T, f *bpsFixture, conn *websocket.Conn) []byte {
	t.Helper()
	var m struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Broker    string `json:"broker"`
		ID        string `json:"id"`
	}
	if err := conn.ReadJSON(&m); err != nil {
		t.Fatal(err)
	}
	if m.Type != "challenge" || m.Broker != f.broker.String() || m.ID != hex.EncodeToString(f.id) {
		t.Fatalf("unexpected challenge message %+v", m)
	}
	challenge, err := hex.DecodeString(m.Challenge)
	if err != nil || !bytes.Equal(challenge, f.sess.challenge) {
		t.Fatalf("challenge %q, want %x", m.Challenge, f.sess.challenge)
	}
	return challenge
}

func sendClaim(t *testing.T, conn *websocket.Conn, sig string) {
	t.Helper()
	if err := conn.WriteJSON(map[string]string{"type": "claim", "signature": sig}); err != nil {
		t.Fatal(err)
	}
}

func expectClaimSent(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	var m struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&m); err != nil {
		t.Fatal(err)
	}
	if m.Type != "claim_sent" {
		t.Fatalf("got %q, want claim_sent", m.Type)
	}
}

// claimed dials publish and completes the claim handshake.
func claimed(t *testing.T, f *bpsFixture) *websocket.Conn {
	t.Helper()
	conn := f.dial(t, "publish")
	challenge := readChallenge(t, f, conn)
	sendClaim(t, conn, f.claimSignature(t, f.signer, challenge))
	expectClaimSent(t, conn)
	return conn
}

func TestBPSPublish(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := claimed(t, f)

	claim := <-f.sess.claims
	if !soc.Valid(swarm.NewChunk(f.addr, claim)) {
		t.Fatal("claim passed to session is not a valid soc at the topic address")
	}
	sc, err := soc.FromChunk(swarm.NewChunk(f.addr, claim))
	if err != nil {
		t.Fatal(err)
	}
	wantPayload := append(append([]byte{}, f.sess.challenge...), f.broker.Bytes()...)
	if got := sc.WrappedChunk().Data()[swarm.SpanSize:]; !bytes.Equal(got, wantPayload) {
		t.Fatalf("claim payload %x, want %x", got, wantPayload)
	}

	msg := f.signedSOC(t, f.signer, []byte("broadcast"))
	if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-f.sess.published:
		if !bytes.Equal(got, msg) {
			t.Fatalf("published %x, want %x", got, msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for publish")
	}
}

func TestBPSPublishClaimErrors(t *testing.T) {
	t.Parallel()

	other, _ := crypto.GenerateSecp256k1Key()
	otherSigner := crypto.NewDefaultSigner(other)

	for _, tc := range []struct {
		name string
		send func(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte)
		code int
	}{
		{
			name: "wrong signer",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte) {
				sendClaim(t, conn, f.claimSignature(t, otherSigner, challenge))
			},
			code: api.BPSCloseInvalidClaim,
		},
		{
			name: "wrong challenge",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, _ []byte) {
				sendClaim(t, conn, f.claimSignature(t, f.signer, make([]byte, 32)))
			},
			code: api.BPSCloseInvalidClaim,
		},
		{
			name: "malformed json",
			send: func(t *testing.T, _ *bpsFixture, conn *websocket.Conn, _ []byte) {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("{not json"))
			},
			code: api.BPSCloseInvalidClaim,
		},
		{
			name: "short signature",
			send: func(t *testing.T, _ *bpsFixture, conn *websocket.Conn, _ []byte) {
				sendClaim(t, conn, "abcd")
			},
			code: api.BPSCloseInvalidClaim,
		},
		{
			name: "0x-prefixed signature",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte) {
				sendClaim(t, conn, "0x"+f.claimSignature(t, f.signer, challenge))
			},
			code: api.BPSCloseInvalidClaim,
		},
		{
			name: "wrong message type",
			send: func(t *testing.T, _ *bpsFixture, conn *websocket.Conn, _ []byte) {
				_ = conn.WriteJSON(map[string]string{"type": "hello"})
			},
			code: api.BPSCloseInvalidMessage,
		},
		{
			name: "binary before claim",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, _ []byte) {
				_ = conn.WriteMessage(websocket.BinaryMessage, f.signedSOC(t, f.signer, []byte("early")))
			},
			code: api.BPSCloseInvalidMessage,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newBPSFixture(t)
			conn := f.dial(t, "publish")
			challenge := readChallenge(t, f, conn)
			tc.send(t, f, conn, challenge)
			expectClose(t, conn, tc.code)
			waitSessionClosed(t, f)
			if len(f.sess.claims) != 0 {
				t.Fatal("rejected claim reached the session")
			}
		})
	}
}

func TestBPSPublishAfterClaimErrors(t *testing.T) {
	t.Parallel()

	t.Run("invalid soc closes 4002", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := claimed(t, f)
		other, _ := crypto.GenerateSecp256k1Key()
		_ = conn.WriteMessage(websocket.BinaryMessage, f.signedSOC(t, crypto.NewDefaultSigner(other), []byte("forged")))
		expectClose(t, conn, api.BPSCloseInvalidMessage)
		if len(f.sess.published) != 0 {
			t.Fatal("invalid soc was published")
		}
	})

	t.Run("text frame closes 4002", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := claimed(t, f)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"claim"}`))
		expectClose(t, conn, api.BPSCloseInvalidMessage)
	})

	t.Run("broker gone closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := claimed(t, f)
		f.sess.end(errors.New("stream reset"))
		expectClose(t, conn, api.BPSCloseBrokerGone)
		waitSessionClosed(t, f)
	})

	t.Run("claim write error closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		f.sess.claimErr = errors.New("write failed")
		conn := f.dial(t, "publish")
		challenge := readChallenge(t, f, conn)
		sendClaim(t, conn, f.claimSignature(t, f.signer, challenge))
		expectClose(t, conn, api.BPSCloseBrokerGone)
	})

	t.Run("publish error closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		f.sess.publishErr = errors.New("write failed")
		conn := claimed(t, f)
		_ = conn.WriteMessage(websocket.BinaryMessage, f.signedSOC(t, f.signer, []byte("x")))
		expectClose(t, conn, api.BPSCloseBrokerGone)
	})
}

func TestBPSPublishClientGoneBeforeClaim(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dial(t, "publish")
	readChallenge(t, f, conn)
	// Send a close frame rather than Close, which the dial cleanup would
	// report as an error on the already closed connection.
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	select {
	case <-f.sess.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("session not closed after client went away")
	}
}

func TestBPSPublishOversizedFrame(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := claimed(t, f)
	_ = conn.WriteMessage(websocket.BinaryMessage, make([]byte, 5000))
	expectClose(t, conn, websocket.CloseMessageTooBig)
	waitSessionClosed(t, f)
}

// Not parallel: mutates the package-level claim timeout.
func TestBPSPublishClaimTimeout(t *testing.T) {
	restore := api.SetBPSClaimTimeout(100 * time.Millisecond)
	defer restore()
	f := newBPSFixture(t)
	conn := f.dial(t, "publish")
	readChallenge(t, f, conn)
	expectClose(t, conn, api.BPSCloseInvalidMessage)
}

func TestBPSPublishInvalidChallengeLength(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	f.sess.challenge = make([]byte, 100)
	conn := f.dial(t, "publish")
	// expectClose skips data frames, so check explicitly that the first
	// frame is the close and no challenge was sent.
	_, _, err := conn.ReadMessage()
	var ce *websocket.CloseError
	if !errors.As(err, &ce) {
		t.Fatalf("want close error, got %v", err)
	}
	if ce.Code != api.BPSCloseBrokerGone || ce.Text != "invalid challenge" {
		t.Fatalf("got close %d %q", ce.Code, ce.Text)
	}
	waitSessionClosed(t, f)
}

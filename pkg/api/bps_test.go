// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bps"
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

	mu  sync.Mutex
	req bps.JoinRequest
}

func (f *fakeBPS) Join(_ context.Context, req bps.JoinRequest) (bps.Session, error) {
	f.mu.Lock()
	f.req = req
	f.mu.Unlock()
	if f.joinErr != nil {
		return nil, f.joinErr
	}
	return f.sess, nil
}

type published struct {
	kind  bps.Kind
	index uint64
	soc   []byte
}

type fakeSession struct {
	challenge []byte
	msgs      chan bps.Message
	done      chan struct{}
	published chan published
	closed    chan struct{}

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
		msgs:      make(chan bps.Message),
		done:      make(chan struct{}),
		published: make(chan published, 8),
		closed:    make(chan struct{}),
	}
}

func (f *fakeSession) Challenge() []byte            { return f.challenge }
func (f *fakeSession) Messages() <-chan bps.Message { return f.msgs }
func (f *fakeSession) Cursor() uint64               { return 0 }
func (f *fakeSession) Done() <-chan struct{}        { return f.done }
func (f *fakeSession) Err() error                   { return f.err }

func (f *fakeSession) Publish(_ context.Context, kind bps.Kind, index uint64, b []byte) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published <- published{kind: kind, index: index, soc: b}
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
	identity []byte
	topic    []byte
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
	identity := make([]byte, 20)
	copy(identity, "bps-test-subscriber")
	sess := newFakeSession()
	f := &bpsFixture{
		bps:      &fakeBPS{sess: sess},
		sess:     sess,
		signer:   signer,
		owner:    owner.Bytes(),
		identity: identity,
		topic:    topic,
		broker:   swarm.RandAddress(t),
		self:     swarm.RandAddress(t),
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
	q := "broker=" + f.broker.String() + "&identity=" + hex.EncodeToString(f.identity)
	u := url.URL{Scheme: "ws", Host: f.listener, Path: f.path(endpoint), RawQuery: q}
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

// signedSOC returns the SOC bytes of the session feed update of kind at index,
// signed by signer under challenge.
func (f *bpsFixture) signedSOC(t *testing.T, signer crypto.Signer, kind bps.Kind, challenge []byte, index uint64, payload []byte) []byte {
	t.Helper()
	id, err := bps.ID(kind, f.topic, challenge, index)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := cac.New(payload)
	if err != nil {
		t.Fatal(err)
	}
	c, err := soc.New(id, ch).Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	return c.Data()
}

// frame returns a publish frame kind | index | soc.
func frame(kind bps.Kind, index uint64, chunk []byte) []byte {
	b := []byte{byte(kind)}
	b = binary.BigEndian.AppendUint64(b, index)
	return append(b, chunk...)
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

	t.Run("forwards delivery", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t, "subscribe")
		chunk := f.signedSOC(t, f.signer, bps.KindData, f.sess.challenge, 7, []byte("hello cohort"))
		f.sess.msgs <- bps.Message{SOC: chunk, Challenge: f.sess.challenge, Index: 7}
		typ, got, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		want := append(append([]byte{}, f.sess.challenge...), 0, 0, 0, 0, 0, 0, 0, 7)
		want = append(want, chunk...)
		if typ != websocket.BinaryMessage || !bytes.Equal(got, want) {
			t.Fatalf("got type %d %x, want binary %x", typ, got, want)
		}
		f.bps.mu.Lock()
		req := f.bps.req
		f.bps.mu.Unlock()
		if !req.Broker.Equal(f.broker) || !bytes.Equal(req.Spec.Principal, f.owner) || !bytes.Equal(req.Identity, f.identity) || !bytes.Equal(req.Spec.Topic, f.topic) {
			t.Fatalf("unexpected join request %+v", req)
		}
	})

	t.Run("missing identity", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		jsonhttptest.Request(t, f.client, http.MethodGet, f.path("subscribe")+"?broker="+f.broker.String(), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code: http.StatusBadRequest, Message: "missing identity",
			}),
		)
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
		DataTopic string `json:"dataTopic"`
		AuthTopic string `json:"authTopic"`
	}
	if err := conn.ReadJSON(&m); err != nil {
		t.Fatal(err)
	}
	dataTopic, _ := bps.SessionTopic(bps.KindData, f.topic, f.sess.challenge)
	authTopic, _ := bps.SessionTopic(bps.KindAuth, f.topic, f.sess.challenge)
	if m.Type != "challenge" || m.Broker != f.broker.String() ||
		m.DataTopic != hex.EncodeToString(dataTopic) || m.AuthTopic != hex.EncodeToString(authTopic) {
		t.Fatalf("unexpected challenge message %+v", m)
	}
	challenge, err := hex.DecodeString(m.Challenge)
	if err != nil || !bytes.Equal(challenge, f.sess.challenge) {
		t.Fatalf("challenge %q, want %x", m.Challenge, f.sess.challenge)
	}
	return challenge
}

func expectPublished(t *testing.T, f *bpsFixture, kind bps.Kind, index uint64, chunk []byte) {
	t.Helper()
	select {
	case got := <-f.sess.published:
		if got.kind != kind || got.index != index || !bytes.Equal(got.soc, chunk) {
			t.Fatalf("published %d/%d %x, want %d/%d %x", got.kind, got.index, got.soc, kind, index, chunk)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for publish")
	}
}

func TestBPSPublish(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dial(t, "publish")
	challenge := readChallenge(t, f, conn)

	f.bps.mu.Lock()
	req := f.bps.req
	f.bps.mu.Unlock()
	if !bytes.Equal(req.Identity, f.owner) {
		t.Fatalf("publisher joined as %x, want the principal %x", req.Identity, f.owner)
	}

	auth := f.signedSOC(t, f.signer, bps.KindAuth, challenge, 0, nil)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindAuth, 0, auth)); err != nil {
		t.Fatal(err)
	}
	expectPublished(t, f, bps.KindAuth, 0, auth)

	data := f.signedSOC(t, f.signer, bps.KindData, challenge, 3, []byte("broadcast"))
	if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 3, data)); err != nil {
		t.Fatal(err)
	}
	expectPublished(t, f, bps.KindData, 3, data)
}

func TestBPSPublishFrameErrors(t *testing.T) {
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
				t.Helper()
				_ = conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 0, f.signedSOC(t, otherSigner, bps.KindData, challenge, 0, []byte("x"))))
			},
			code: api.BPSCloseInvalidSOC,
		},
		{
			name: "wrong challenge",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, _ []byte) {
				t.Helper()
				_ = conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 0, f.signedSOC(t, f.signer, bps.KindData, make([]byte, 32), 0, []byte("x"))))
			},
			code: api.BPSCloseInvalidSOC,
		},
		{
			name: "index not the signed one",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte) {
				t.Helper()
				_ = conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 1, f.signedSOC(t, f.signer, bps.KindData, challenge, 0, []byte("x"))))
			},
			code: api.BPSCloseInvalidSOC,
		},
		{
			name: "auth relabelled as data",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte) {
				t.Helper()
				_ = conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 0, f.signedSOC(t, f.signer, bps.KindAuth, challenge, 0, nil)))
			},
			code: api.BPSCloseInvalidSOC,
		},
		{
			name: "auth with payload",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte) {
				t.Helper()
				_ = conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindAuth, 0, f.signedSOC(t, f.signer, bps.KindAuth, challenge, 0, []byte("x"))))
			},
			code: api.BPSCloseInvalidSOC,
		},
		{
			name: "unknown kind",
			send: func(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte) {
				t.Helper()
				_ = conn.WriteMessage(websocket.BinaryMessage, frame(bps.Kind(9), 0, f.signedSOC(t, f.signer, bps.KindData, challenge, 0, []byte("x"))))
			},
			code: api.BPSCloseInvalidMessage,
		},
		{
			name: "short frame",
			send: func(t *testing.T, _ *bpsFixture, conn *websocket.Conn, _ []byte) {
				t.Helper()
				_ = conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2})
			},
			code: api.BPSCloseInvalidMessage,
		},
		{
			name: "text frame",
			send: func(t *testing.T, _ *bpsFixture, conn *websocket.Conn, _ []byte) {
				t.Helper()
				_ = conn.WriteJSON(map[string]string{"type": "auth"})
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
			if len(f.sess.published) != 0 {
				t.Fatal("rejected frame reached the session")
			}
		})
	}
}

func TestBPSPublishSessionErrors(t *testing.T) {
	t.Parallel()

	t.Run("broker gone closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t, "publish")
		readChallenge(t, f, conn)
		f.sess.end(errors.New("stream reset"))
		expectClose(t, conn, api.BPSCloseBrokerGone)
		waitSessionClosed(t, f)
	})

	t.Run("publish error closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		f.sess.publishErr = errors.New("write failed")
		conn := f.dial(t, "publish")
		challenge := readChallenge(t, f, conn)
		_ = conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 0, f.signedSOC(t, f.signer, bps.KindData, challenge, 0, []byte("x"))))
		expectClose(t, conn, api.BPSCloseBrokerGone)
	})
}

func TestBPSPublishClientGoneBeforeAuth(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dial(t, "publish")
	readChallenge(t, f, conn)
	// Send a close frame rather than Close, which the dial cleanup would
	// report as an error on the already closed connection.
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	waitSessionClosed(t, f)
}

func TestBPSPublishOversizedFrame(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dial(t, "publish")
	readChallenge(t, f, conn)
	_ = conn.WriteMessage(websocket.BinaryMessage, make([]byte, 5000))
	expectClose(t, conn, websocket.CloseMessageTooBig)
	waitSessionClosed(t, f)
}

// Not parallel: mutates the package-level auth timeout.
func TestBPSPublishAuthTimeout(t *testing.T) {
	restore := api.SetBPSAuthTimeout(100 * time.Millisecond)
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

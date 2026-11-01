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
	"strings"
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
	// as the session does: the principal needs no cue
	if bytes.Equal(req.Identity, req.Spec.Principal) {
		close(f.sess.granted)
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
	granted   chan struct{}
	done      chan struct{}
	published chan published
	closed    <-chan struct{}    // closed by Close
	cancel    context.CancelFunc // Close, as the session's: a request, any number of times

	publishErr error
	err        error
}

func newFakeSession() *fakeSession {
	challenge := make([]byte, 32)
	copy(challenge, "bps-test-challenge")
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeSession{
		challenge: challenge,
		msgs:      make(chan bps.Message),
		granted:   make(chan struct{}),
		done:      make(chan struct{}),
		published: make(chan published, 8),
		closed:    ctx.Done(),
		cancel:    cancel,
	}
}

func (f *fakeSession) Challenge() []byte            { return f.challenge }
func (f *fakeSession) Messages() <-chan bps.Message { return f.msgs }
func (f *fakeSession) Position() bps.Position       { return bps.Position{} }
func (f *fakeSession) Granted() <-chan struct{}     { return f.granted }
func (f *fakeSession) Done() <-chan struct{}        { return f.done }
func (f *fakeSession) Err() error                   { return f.err }

// Publish refuses, as the session does, what comes before the grant. The rest
// of what the session refuses is told by publishErr.
func (f *fakeSession) Publish(_ context.Context, kind bps.Kind, index uint64, b []byte) error {
	select {
	case <-f.granted:
	default:
		return bps.ErrNotPublisher
	}
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published <- published{kind: kind, index: index, soc: b}
	return nil
}

func (f *fakeSession) Close() error {
	f.cancel()
	return nil
}

// end simulates the broker stream ending with err; a test calls it once.
func (f *fakeSession) end(err error) {
	f.err = err
	close(f.done)
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

func (f *bpsFixture) path() string {
	return fmt.Sprintf("/bps/%s/%s", hex.EncodeToString(f.owner), hex.EncodeToString(f.topic))
}

// dial opens the socket as the fixture's identity, which listens.
func (f *bpsFixture) dial(t *testing.T) *websocket.Conn {
	t.Helper()
	return f.dialQuery(t, f.identity, "")
}

// dialOwner opens the socket as the owner.
func (f *bpsFixture) dialOwner(t *testing.T) *websocket.Conn {
	t.Helper()
	return f.dialQuery(t, f.owner, "")
}

// dialQuery opens the socket as identity, with more query parameters.
func (f *bpsFixture) dialQuery(t *testing.T, identity []byte, query string) *websocket.Conn {
	t.Helper()
	q := "broker=" + f.broker.String() + "&identity=" + hex.EncodeToString(identity) + query
	u := url.URL{Scheme: "ws", Host: f.listener, Path: f.path(), RawQuery: q}
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

// authenticate sends the session's AUTH and waits for the bridge to forward it.
func authenticate(t *testing.T, f *bpsFixture, conn *websocket.Conn, challenge []byte) {
	t.Helper()
	auth := f.signedSOC(t, f.signer, bps.KindAuth, challenge, 0, nil)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindAuth, 0, auth)); err != nil {
		t.Fatal(err)
	}
	expectPublished(t, f, bps.KindAuth, 0, auth)
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

// deliver hands the bridge a delivery, as the session does.
func deliver(t *testing.T, f *bpsFixture, msg bps.Message) {
	t.Helper()
	select {
	case f.sess.msgs <- msg:
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge takes no delivery")
	}
}

// expectOpen checks that the socket stays open for d: a delivery still arrives.
func expectOpen(t *testing.T, f *bpsFixture, conn *websocket.Conn, d time.Duration) {
	t.Helper()
	time.Sleep(d)
	deliver(t, f, bps.Message{Kind: bps.KindData, Owner: f.owner, SOC: []byte("soc"), Challenge: f.sess.challenge, Index: 1})
	for {
		typ, _, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("socket closed: %v", err)
		}
		if typ == websocket.BinaryMessage {
			return
		}
	}
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

// bpsChallenge is the challenge message as a dApp reads it.
type bpsChallenge struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Broker      string `json:"broker"`
	DataTopic   string `json:"dataTopic"`
	AuthTopic   string `json:"authTopic"`
	RosterTopic string `json:"rosterTopic"`
	EOSTopic    string `json:"eosTopic"`
}

// readChallenge reads the challenge frame and checks the fields every stream gets.
func readChallenge(t *testing.T, f *bpsFixture, conn *websocket.Conn) bpsChallenge {
	t.Helper()
	var m bpsChallenge
	if err := conn.ReadJSON(&m); err != nil {
		t.Fatal(err)
	}
	dataTopic, _ := bps.SessionTopic(bps.KindData, f.topic, f.sess.challenge)
	authTopic, _ := bps.SessionTopic(bps.KindAuth, f.topic, f.sess.challenge)
	if m.Type != "challenge" || m.Broker != f.broker.String() ||
		m.DataTopic != hex.EncodeToString(dataTopic) || m.AuthTopic != hex.EncodeToString(authTopic) {
		t.Fatalf("unexpected challenge message %+v", m)
	}
	if m.Challenge != hex.EncodeToString(f.sess.challenge) {
		t.Fatalf("challenge %q, want %x", m.Challenge, f.sess.challenge)
	}
	return m
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

func joinRequest(f *bpsFixture) bps.JoinRequest {
	f.bps.mu.Lock()
	defer f.bps.mu.Unlock()
	return f.bps.req
}

func TestBPSPreUpgrade(t *testing.T) {
	t.Parallel()

	f := newBPSFixture(t)
	nilClient, _, _, _, _ := newTestServer(t, testServerOptions{})
	owner := hex.EncodeToString(f.owner)
	topic := hex.EncodeToString(f.topic)
	identity := "&identity=" + hex.EncodeToString(f.identity)
	base := fmt.Sprintf("/bps/%s/%s", owner, topic)

	jsonhttptest.Request(t, f.client, http.MethodGet, base, http.StatusBadRequest,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Code: http.StatusBadRequest, Message: "missing broker",
		}),
	)
	jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker=zz"+identity, http.StatusBadRequest)
	jsonhttptest.Request(t, f.client, http.MethodGet,
		fmt.Sprintf("/bps/%s/%s?broker=%s%s", "zz", topic, f.broker, identity), http.StatusBadRequest)
	jsonhttptest.Request(t, f.client, http.MethodGet,
		fmt.Sprintf("/bps/%s/%s?broker=%s%s", owner, "abcd", f.broker, identity), http.StatusBadRequest)
	jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker=abcd"+identity, http.StatusBadRequest,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Code: http.StatusBadRequest, Message: "invalid broker",
		}),
	)
	jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker="+f.self.String()+identity, http.StatusBadRequest,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Code: http.StatusBadRequest, Message: "broker cannot be this node",
		}),
	)
	jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker="+f.broker.String(), http.StatusBadRequest,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Code: http.StatusBadRequest, Message: "missing identity",
		}),
	)
	jsonhttptest.Request(t, f.client, http.MethodGet, base+"?broker="+f.broker.String()+"&identity=abcd", http.StatusBadRequest)
	jsonhttptest.Request(t, nilClient, http.MethodGet, base+"?broker="+f.broker.String()+identity, http.StatusServiceUnavailable)
}

// TestBPSListener: a stream declaring any address but the owner's listens.
func TestBPSListener(t *testing.T) {
	t.Parallel()

	t.Run("forwards deliveries", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t)
		if m := readChallenge(t, f, conn); m.RosterTopic != "" || m.EOSTopic != "" {
			t.Fatalf("the service topics are the owner's: %+v", m)
		}
		chunk := f.signedSOC(t, f.signer, bps.KindData, f.sess.challenge, 7, []byte("hello cohort"))
		deliver(t, f, bps.Message{Kind: bps.KindData, Owner: f.owner, SOC: chunk, Challenge: f.sess.challenge, Index: 7})
		typ, got, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		want := append([]byte{byte(bps.KindData)}, f.owner...)
		want = append(want, f.sess.challenge...)
		want = append(want, 0, 0, 0, 0, 0, 0, 0, 7)
		want = append(want, chunk...)
		if typ != websocket.BinaryMessage || !bytes.Equal(got, want) {
			t.Fatalf("got type %d %x, want binary %x", typ, got, want)
		}
		req := joinRequest(f)
		if !req.Broker.Equal(f.broker) || !bytes.Equal(req.Spec.Principal, f.owner) || !bytes.Equal(req.Identity, f.identity) || !bytes.Equal(req.Spec.Topic, f.topic) {
			t.Fatalf("unexpected join request %+v", req)
		}
	})

	t.Run("a frame before the grant closes 4002", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t)
		readChallenge(t, f, conn)
		auth := f.signedSOC(t, f.signer, bps.KindAuth, f.sess.challenge, 0, nil)
		if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindAuth, 0, auth)); err != nil {
			t.Fatal(err)
		}
		expectClose(t, conn, api.BPSCloseInvalidMessage)
		waitSessionClosed(t, f)
	})

	t.Run("broker gone closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t)
		readChallenge(t, f, conn)
		f.sess.end(errors.New("stream reset"))
		expectClose(t, conn, api.BPSCloseBrokerGone)
		waitSessionClosed(t, f)
	})

	t.Run("join error closes 4003", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		f.bps.joinErr = errors.New("broker unreachable")
		conn := f.dial(t)
		expectClose(t, conn, api.BPSCloseBrokerGone)
	})

	t.Run("end of stream closes 4004", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dial(t)
		readChallenge(t, f, conn)
		f.sess.end(bps.ErrEndOfStream)
		expectClose(t, conn, api.BPSCloseEndOfStream)
	})
}

// TestBPSGrant: a listener is told when a roster naming it has reached its
// stream, and may then send its AUTH and publish.
func TestBPSGrant(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	key, _ := crypto.GenerateSecp256k1Key()
	seatSigner := crypto.NewDefaultSigner(key)
	seatAddr, _ := seatSigner.EthereumAddress()
	seat := seatAddr.Bytes()

	conn := f.dialQuery(t, seat, "")
	readChallenge(t, f, conn)
	if req := joinRequest(f); !bytes.Equal(req.Identity, seat) {
		t.Fatalf("unexpected join request %+v", req)
	}

	// listening until the grant
	deliver(t, f, bps.Message{Kind: bps.KindData, Owner: f.owner, SOC: []byte("soc"), Challenge: f.sess.challenge})
	if typ, _, err := conn.ReadMessage(); err != nil || typ != websocket.BinaryMessage {
		t.Fatalf("want a delivery, got %d %v", typ, err)
	}

	close(f.sess.granted)
	typ, msg, err := conn.ReadMessage()
	if err != nil || typ != websocket.TextMessage || string(msg) != api.BPSGranted {
		t.Fatalf("want granted, got %d %q %v", typ, msg, err)
	}
	auth := f.signedSOC(t, seatSigner, bps.KindAuth, f.sess.challenge, 0, nil)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindAuth, 0, auth)); err != nil {
		t.Fatal(err)
	}
	expectPublished(t, f, bps.KindAuth, 0, auth)
	data := f.signedSOC(t, seatSigner, bps.KindData, f.sess.challenge, 0, []byte("from the seat"))
	if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 0, data)); err != nil {
		t.Fatal(err)
	}
	expectPublished(t, f, bps.KindData, 0, data)
}

// TestBPSOwner: a stream declaring the owner's address publishes, service
// messages included, and receives what the others publish.
func TestBPSOwner(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dialOwner(t)
	m := readChallenge(t, f, conn)
	rosterTopic, _ := bps.SessionTopic(bps.KindRoster, f.topic, f.sess.challenge)
	eosTopic, _ := bps.SessionTopic(bps.KindEOS, f.topic, f.sess.challenge)
	if m.RosterTopic != hex.EncodeToString(rosterTopic) || m.EOSTopic != hex.EncodeToString(eosTopic) {
		t.Fatalf("unexpected service topics %+v", m)
	}
	if req := joinRequest(f); !bytes.Equal(req.Identity, f.owner) {
		t.Fatalf("joined as %x, want the principal %x", req.Identity, f.owner)
	}
	authenticate(t, f, conn, f.sess.challenge)

	data := f.signedSOC(t, f.signer, bps.KindData, f.sess.challenge, 3, []byte("broadcast"))
	if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindData, 3, data)); err != nil {
		t.Fatal(err)
	}
	expectPublished(t, f, bps.KindData, 3, data)

	ros := f.signedSOC(t, f.signer, bps.KindRoster, f.sess.challenge, 0, nil)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindRoster, 0, ros)); err != nil {
		t.Fatal(err)
	}
	expectPublished(t, f, bps.KindRoster, 0, ros)

	seat := make([]byte, 20)
	copy(seat, "bps-test-seat")
	deliver(t, f, bps.Message{Kind: bps.KindData, Owner: seat, SOC: []byte("soc"), Challenge: f.sess.challenge, Index: 1})
	typ, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.BinaryMessage || got[0] != byte(bps.KindData) || !bytes.Equal(got[1:21], seat) {
		t.Fatalf("got type %d %x", typ, got)
	}
}

// TestBPSFrameErrors: what is wrong with a frame as such is the bridge's to say.
func TestBPSFrameErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		send func(conn *websocket.Conn) error
	}{
		{"short frame", func(conn *websocket.Conn) error { return conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}) }},
		{"text frame", func(conn *websocket.Conn) error { return conn.WriteJSON(map[string]string{"type": "auth"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newBPSFixture(t)
			conn := f.dialOwner(t)
			readChallenge(t, f, conn)
			if err := tc.send(conn); err != nil {
				t.Fatal(err)
			}
			expectClose(t, conn, api.BPSCloseInvalidMessage)
			waitSessionClosed(t, f)
			if len(f.sess.published) != 0 {
				t.Fatal("rejected frame reached the session")
			}
		})
	}
}

// TestBPSPublishErrors: what the session refuses closes the socket with the code
// its error maps to.
func TestBPSPublishErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		code int
	}{
		{fmt.Errorf("id not derived from frame: %w", bps.ErrInvalidSOC), api.BPSCloseInvalidSOC},
		{fmt.Errorf("data before the auth: %w", bps.ErrInvalidMessage), api.BPSCloseInvalidMessage},
		{fmt.Errorf("the admin must not be on the roster: %w", bps.ErrInvalidRoster), api.BPSCloseInvalidMessage},
		{bps.ErrNotPublisher, api.BPSCloseInvalidMessage},
		{errors.New("write failed"), api.BPSCloseBrokerGone},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			t.Parallel()
			f := newBPSFixture(t)
			f.sess.publishErr = tc.err
			conn := f.dialOwner(t)
			readChallenge(t, f, conn)
			auth := f.signedSOC(t, f.signer, bps.KindAuth, f.sess.challenge, 0, nil)
			if err := conn.WriteMessage(websocket.BinaryMessage, frame(bps.KindAuth, 0, auth)); err != nil {
				t.Fatal(err)
			}
			expectClose(t, conn, tc.code)
			waitSessionClosed(t, f)
		})
	}
}

func TestBPSCloseCode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		code int
	}{
		{bps.ErrInvalidSOC, api.BPSCloseInvalidSOC},
		{api.ErrBPSInvalidMessage, api.BPSCloseInvalidMessage},
		{bps.ErrInvalidMessage, api.BPSCloseInvalidMessage},
		{bps.ErrInvalidRoster, api.BPSCloseInvalidMessage},
		{bps.ErrNotPublisher, api.BPSCloseInvalidMessage},
		{api.ErrBPSBrokerGone, api.BPSCloseBrokerGone},
		{errors.New("stream reset"), api.BPSCloseBrokerGone},
		{bps.ErrEndOfStream, api.BPSCloseEndOfStream},
		{fmt.Errorf("wrapped: %w", bps.ErrEndOfStream), api.BPSCloseEndOfStream},
		{api.ErrBPSAuthTimeout, api.BPSCloseAuthTimeout},
		{api.ErrBPSShutdown, websocket.CloseGoingAway},
	} {
		if got := api.BPSCloseCode(tc.err); got != tc.code {
			t.Errorf("%v: got %d, want %d", tc.err, got, tc.code)
		}
	}
}

func TestBPSClientGone(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dialOwner(t)
	readChallenge(t, f, conn)
	// Send a close frame rather than Close, which the dial cleanup would
	// report as an error on the already closed connection.
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	waitSessionClosed(t, f)
}

func TestBPSOversizedFrame(t *testing.T) {
	t.Parallel()
	f := newBPSFixture(t)
	conn := f.dialOwner(t)
	readChallenge(t, f, conn)
	_ = conn.WriteMessage(websocket.BinaryMessage, make([]byte, 5000))
	expectClose(t, conn, websocket.CloseMessageTooBig)
	waitSessionClosed(t, f)
}

// Not parallel: mutates the package-level auth timeout.
func TestBPSAuthTimeout(t *testing.T) {
	const timeout = 100 * time.Millisecond
	restore := api.SetBPSAuthTimeout(timeout)
	defer restore()

	t.Run("the owner's AUTH overdue closes 4005", func(t *testing.T) {
		f := newBPSFixture(t)
		conn := f.dialOwner(t)
		readChallenge(t, f, conn)
		expectClose(t, conn, api.BPSCloseAuthTimeout)
		waitSessionClosed(t, f)
	})

	t.Run("the owner's AUTH in time stops the deadline", func(t *testing.T) {
		f := newBPSFixture(t)
		conn := f.dialOwner(t)
		readChallenge(t, f, conn)
		authenticate(t, f, conn, f.sess.challenge)
		expectOpen(t, f, conn, 2*timeout)
	})

	t.Run("a listener has no deadline, granted or not", func(t *testing.T) {
		f := newBPSFixture(t)
		conn := f.dial(t)
		readChallenge(t, f, conn)
		expectOpen(t, f, conn, 2*timeout)
		close(f.sess.granted)
		expectOpen(t, f, conn, 2*timeout)
	})
}

func TestBPSPosition(t *testing.T) {
	t.Parallel()
	seat1 := bytes.Repeat([]byte{0x11}, 20)
	seat2 := bytes.Repeat([]byte{0x22}, 20)

	t.Run("cursor, roster and rosterCursor reach the join", func(t *testing.T) {
		t.Parallel()
		f := newBPSFixture(t)
		conn := f.dialQuery(t, f.identity, "&cursor=7&rosterCursor=3&roster="+hex.EncodeToString(seat1)+":4,0x"+hex.EncodeToString(seat2)+":0")
		readChallenge(t, f, conn)
		pos := joinRequest(f).Position
		if pos.RosterCursor != 3 || len(pos.Roster) != 2 || !bytes.Equal(pos.Roster[0], seat1) || !bytes.Equal(pos.Roster[1], seat2) {
			t.Fatalf("roster %x at %d", pos.Roster, pos.RosterCursor)
		}
		if pos.Cursors[string(f.owner)] != 7 || pos.Cursors[string(seat1)] != 4 || pos.Cursors[string(seat2)] != 0 || len(pos.Cursors) != 3 {
			t.Fatalf("cursors %v", pos.Cursors)
		}
	})

	for _, tc := range []struct {
		name, roster string
	}{
		{"no index", hex.EncodeToString(seat1)},
		{"short address", "1122:4"},
		{"bad index", hex.EncodeToString(seat1) + ":x"},
		{"the owner", "OWNER:1"},
		{"twice", hex.EncodeToString(seat1) + ":1," + hex.EncodeToString(seat1) + ":2"},
		{"too many", strings.Repeat(hex.EncodeToString(seat1)+":1,", bps.MaxRosterSize) + hex.EncodeToString(seat2) + ":1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newBPSFixture(t)
			roster := strings.ReplaceAll(tc.roster, "OWNER", hex.EncodeToString(f.owner))
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, f.path()+"?broker="+f.broker.String()+"&identity="+hex.EncodeToString(f.identity)+"&roster="+roster, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := f.client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", resp.StatusCode)
			}
		})
	}
}

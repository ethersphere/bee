// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/gsoc"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/soc"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/gorilla/websocket"
)

// TestGsocWebsocketSingleHandler creates a single websocket handler on a chunk address, and receives a message
func TestGsocWebsocketSingleHandler(t *testing.T) {
	t.Parallel()

	var (
		id               = make([]byte, 32)
		g, cl, signer, _ = newGsocTest(t, id, 0)
		respC            = make(chan error, 1)
		payload          = []byte("hello there!")
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	ch, _ = socCh.Sign(signer)
	socCh, _ = soc.FromChunk(ch)
	g.Handle(socCh)

	go expectMessage(t, cl, respC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

func TestGsocWebsocketMultiHandler(t *testing.T) {
	t.Parallel()

	var (
		id                      = make([]byte, 32)
		g, cl, signer, listener = newGsocTest(t, make([]byte, 32), 0)
		owner, _                = signer.EthereumAddress()
		chunkAddr, _            = soc.CreateAddress(id, owner.Bytes())
		u                       = url.URL{Scheme: "ws", Host: listener, Path: fmt.Sprintf("/gsoc/subscribe/%s", hex.EncodeToString(chunkAddr.Bytes()))}
		cl2, _, err             = websocket.DefaultDialer.Dial(u.String(), nil)
		respC                   = make(chan error, 2)
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

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	ch, _ = socCh.Sign(signer)
	socCh, _ = soc.FromChunk(ch)

	// close the websocket before calling GSOC with the message
	err = cl.WriteMessage(websocket.CloseMessage, []byte{})
	if err != nil {
		t.Fatal(err)
	}

	g.Handle(socCh)

	go expectMessage(t, cl, respC, payload)
	go expectMessage(t, cl2, respC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

// TestGsocPong tests that the websocket api adheres to the websocket standard
// and sends ping-pong messages to keep the connection alive.
// The test opens a websocket, keeps it alive for 500ms, then receives a GSOC message.
func TestGsocPong(t *testing.T) {
	t.Parallel()
	id := make([]byte, 32)

	var (
		g, cl, signer, _ = newGsocTest(t, id, 90*time.Millisecond)

		respC    = make(chan error, 1)
		pongWait = 1 * time.Millisecond
	)

	cl.SetReadLimit(swarm.ChunkSize)
	err := cl.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond) // wait to see that the websocket is kept alive
	ch, _ := cac.New([]byte("hello there!"))
	socCh := soc.New(id, ch)
	ch, _ = socCh.Sign(signer)
	socCh, _ = soc.FromChunk(ch)

	g.Handle(socCh)

	go expectMessage(t, cl, respC, nil)
	if err := <-respC; err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		// note: error has *websocket.netError type so we need to check error by checking message
		t.Fatal("want timeout error")
	}
}

// TestGsocWebsocketWrappedChunkData verifies that the Swarm-Soc-Fields header
// allows requesting the whole wrapped chunk data (span + payload).
func TestGsocWebsocketWrappedChunkData(t *testing.T) {
	t.Parallel()

	var (
		id                  = make([]byte, 32)
		headers             = http.Header{api.SwarmSocFieldsHeader: []string{"span,payload"}}
		g, cl, signer, _, _ = newGsocTestWithOpts(t, id, 0, headers)
		respC               = make(chan error, 1)
		payload             = []byte("The most dangerous phrase in the language is: ‘We've always done it this way.’")
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	signedCh, _ := socCh.Sign(signer)
	socCh, _ = soc.FromChunk(signedCh)
	g.Handle(socCh)

	// span (8 bytes) + payload == full wrapped chunk data
	go expectMessage(t, cl, respC, ch.Data())
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

// TestGsocWebsocketSocFields verifies that multiple SOC fields are serialized in
// the order they are provided in the Swarm-Soc-Fields header.
func TestGsocWebsocketSocFields(t *testing.T) {
	t.Parallel()

	var (
		id                  = make([]byte, 32)
		headers             = http.Header{api.SwarmSocFieldsHeader: []string{"identifier,wrappedAddress,payload"}}
		g, cl, signer, _, _ = newGsocTestWithOpts(t, id, 0, headers)
		respC               = make(chan error, 1)
		payload             = []byte("The future is already here — it's just not evenly distributed.")
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	signedCh, _ := socCh.Sign(signer)
	socCh, _ = soc.FromChunk(signedCh)
	g.Handle(socCh)

	expected := make([]byte, 0, len(id)+swarm.HashSize+len(payload))
	expected = append(expected, id...)
	expected = append(expected, ch.Address().Bytes()...)
	expected = append(expected, payload...)

	go expectMessage(t, cl, respC, expected)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

// TestGsocWebsocketSocFieldsDeduplication verifies that repeated field names in
// the Swarm-Soc-Fields header are de-duplicated, keeping only the first
// occurrence, instead of serializing the same field multiple times.
func TestGsocWebsocketSocFieldsDeduplication(t *testing.T) {
	t.Parallel()

	var (
		id                  = make([]byte, 32)
		headers             = http.Header{api.SwarmSocFieldsHeader: []string{"payload,payload,identifier,payload,identifier"}}
		g, cl, signer, _, _ = newGsocTestWithOpts(t, id, 0, headers)
		respC               = make(chan error, 1)
		payload             = []byte("Simplicity is the ultimate sophistication.")
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	signedCh, _ := socCh.Sign(signer)
	socCh, _ = soc.FromChunk(signedCh)
	g.Handle(socCh)

	// each requested field must appear exactly once, in first-occurrence order
	expected := make([]byte, 0, len(payload)+len(id))
	expected = append(expected, payload...)
	expected = append(expected, id...)

	go expectMessage(t, cl, respC, expected)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}
}

// TestGsocWebsocketInvalidFieldsHeader verifies that an unknown field name in
// the Swarm-Soc-Fields header is rejected with a 400 Bad Request before the
// websocket upgrade is attempted.
func TestGsocWebsocketInvalidFieldsHeader(t *testing.T) {
	t.Parallel()

	var (
		id         = make([]byte, 32)
		gsocSvc    = gsoc.New(log.Noop)
		addrHex    = hex.EncodeToString(id)
		batchStore = mockbatchstore.New()
		storer     = mockstorer.New()
	)
	testutil.CleanupCloser(t, gsocSvc)

	client, _, _, _ := newTestServer(t, testServerOptions{
		Gsoc:       gsocSvc,
		Storer:     storer,
		BatchStore: batchStore,
		Logger:     log.Noop,
	})

	jsonhttptest.Request(t, client, http.MethodGet, "/gsoc/subscribe/"+addrHex, http.StatusBadRequest,
		jsonhttptest.WithRequestHeader(api.SwarmSocFieldsHeader, "bogusfield"),
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "invalid soc fields header",
			Code:    http.StatusBadRequest,
		}),
	)
}

// TestGsocWebsocketSlowConsumer verifies that when a subscriber cannot keep up
// with incoming GSOC messages, the server closes the connection instead of
// blocking indefinitely or racing on the underlying websocket connection.
//
// The connection is served over an in-memory net.Pipe, which is fully
// synchronous (unbuffered): a write only completes once a matching read
// consumes it. This makes the small dataC buffer overflow deterministically
// as soon as the client stops reading, instead of depending on the size of
// the OS's (possibly very large, auto-tuned) TCP socket buffers.
func TestGsocWebsocketSlowConsumer(t *testing.T) {
	t.Parallel()

	const messageCount = 10

	var (
		id         = make([]byte, 32)
		batchStore = mockbatchstore.New()
		storer     = mockstorer.New()
		gsocSvc    = gsoc.New(log.Noop)
		svc        *api.Service
	)
	testutil.CleanupCloser(t, gsocSvc)

	newTestServer(t, testServerOptions{
		Gsoc:       gsocSvc,
		Storer:     storer,
		BatchStore: batchStore,
		Logger:     log.Noop,
		ServiceOut: &svc,
	})

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	chunkAddr, _ := soc.CreateAddress(id, owner.Bytes())

	ln := newPipeListener()
	srv := &http.Server{Handler: svc}
	testutil.CleanupCloser(t, srv)
	go func() { _ = srv.Serve(ln) }()

	clientConn, serverConn := net.Pipe()
	ln.offer(serverConn)

	u := url.URL{Scheme: "ws", Host: "pipe", Path: "/gsoc/subscribe/" + hex.EncodeToString(chunkAddr.Bytes())}
	dialer := websocket.Dialer{
		NetDial: func(_, _ string) (net.Conn, error) { return clientConn, nil },
	}
	cl, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	testutil.CleanupCloser(t, cl)

	// never read from cl, so the dataC buffer (cap 2) fills up almost
	// immediately: the first message blocks the single writer goroutine
	// (nothing reads the pipe), and the next ones queue up and overflow.
	for i := range messageCount {
		payload := []byte{byte(i)}
		ch, _ := cac.New(payload)
		socCh := soc.New(id, ch)
		signedCh, _ := socCh.Sign(signer)
		socCh, _ = soc.FromChunk(signedCh)
		gsocSvc.Handle(socCh)
	}

	if err := cl.SetReadDeadline(time.Now().Add(longTimeout)); err != nil {
		t.Fatal(err)
	}

	// Drain whatever messages had already been handed to the (synchronous)
	// pipe before the overflow was detected; the connection must eventually
	// be closed instead of the server delivering every message regardless of
	// how far behind the consumer falls.
	var readErr error
	for i := 0; i < messageCount && readErr == nil; i++ {
		_, _, readErr = cl.ReadMessage()
	}
	if readErr == nil {
		t.Fatal("expected connection to be closed for a slow consumer")
	}
}

// pipeListener is a net.Listener that hands out pre-established net.Conn
// pairs, so an http.Server can be driven over an in-memory net.Pipe instead
// of a real OS socket.
type pipeListener struct {
	connCh chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		connCh: make(chan net.Conn, 1),
		closed: make(chan struct{}),
	}
}

func (l *pipeListener) offer(conn net.Conn) { l.connCh <- conn }

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connCh:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// TestGsocWebsocketMessageOrdering verifies that sequential Handle calls for
// the same GSOC address are delivered to the subscriber in the same order.
func TestGsocWebsocketMessageOrdering(t *testing.T) {
	t.Parallel()

	const messageCount = 10

	var (
		id               = make([]byte, 32)
		g, cl, signer, _ = newGsocTest(t, id, 0)
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	payloads := make([][]byte, messageCount)
	for i := range payloads {
		payloads[i] = fmt.Appendf(nil, "message-%d", i)
	}

	for _, payload := range payloads {
		ch, _ := cac.New(payload)
		socCh := soc.New(id, ch)
		signedCh, _ := socCh.Sign(signer)
		socCh, _ = soc.FromChunk(signedCh)
		g.Handle(socCh)
	}

	for i, want := range payloads {
		_, got, err := cl.ReadMessage()
		if err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("message %d: got %q, want %q", i, got, want)
		}
	}
}

// TestGsocWebsocketCacheWrappedChunk verifies that the Swarm-Cache-Wrapped-Chunk
// header causes the wrapped chunk to be stored in the cache so that it can be
// resolved through the bytes endpoint.
func TestGsocWebsocketCacheWrappedChunk(t *testing.T) {
	t.Parallel()

	var (
		id                       = make([]byte, 32)
		headers                  = http.Header{api.SwarmCacheWrappedChunkHeader: []string{"true"}}
		g, cl, signer, _, storer = newGsocTestWithOpts(t, id, 0, headers)
		respC                    = make(chan error, 1)
		payload                  = []byte("If you don't like change, you're going to like irrelevance even less.")
	)

	err := cl.SetReadDeadline(time.Now().Add(longTimeout))
	if err != nil {
		t.Fatal(err)
	}
	cl.SetReadLimit(swarm.ChunkSize)

	ch, _ := cac.New(payload)
	socCh := soc.New(id, ch)
	signedCh, _ := socCh.Sign(signer)
	socCh, _ = soc.FromChunk(signedCh)
	g.Handle(socCh)

	go expectMessage(t, cl, respC, payload)
	if err := <-respC; err != nil {
		t.Fatal(err)
	}

	got, err := storer.ChunkStore().Get(context.Background(), ch.Address())
	if err != nil {
		t.Fatalf("wrapped chunk not cached: %v", err)
	}
	if !bytes.Equal(got.Data(), ch.Data()) {
		t.Fatal("cached wrapped chunk data mismatch")
	}
}

func newGsocTest(t *testing.T, socId []byte, pingPeriod time.Duration) (gsoc.Listener, *websocket.Conn, crypto.Signer, string) {
	t.Helper()
	g, cl, signer, listener, _ := newGsocTestWithOpts(t, socId, pingPeriod, nil)
	return g, cl, signer, listener
}

func newGsocTestWithOpts(t *testing.T, socId []byte, pingPeriod time.Duration, headers http.Header) (gsoc.Listener, *websocket.Conn, crypto.Signer, string, api.Storer) {
	t.Helper()
	if pingPeriod == 0 {
		pingPeriod = 10 * time.Second
	}
	var (
		batchStore = mockbatchstore.New()
		storer     = mockstorer.New()
	)

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	chunkAddr, _ := soc.CreateAddress(socId, owner.Bytes())

	gsoc := gsoc.New(log.NewLogger("test"))
	testutil.CleanupCloser(t, gsoc)

	_, cl, listener, _ := newTestServer(t, testServerOptions{
		Gsoc:         gsoc,
		WsPath:       fmt.Sprintf("/gsoc/subscribe/%s", hex.EncodeToString(chunkAddr.Bytes())),
		WsHeaders:    headers,
		Storer:       storer,
		BatchStore:   batchStore,
		Logger:       log.Noop,
		WsPingPeriod: pingPeriod,
	})

	return gsoc, cl, signer, listener, storer
}

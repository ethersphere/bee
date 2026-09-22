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

// TestGsocWebsocketSlowConsumer verifies that a subscriber that falls behind
// incoming GSOC messages is not dropped or disconnected: the server queues
// every message (no cap) and delivers the full backlog, in order, once the
// consumer catches up, instead of racing on the underlying websocket
// connection or blocking the (synchronous) GSOC handler indefinitely.
//
// The connection is served over an in-memory net.Pipe, which is fully
// synchronous (unbuffered): a write only completes once a matching read
// consumes it, so the server-side writer is deterministically blocked on the
// first message for as long as the client doesn't read, while further
// messages still queue up behind it without being dropped.
func TestGsocWebsocketSlowConsumer(t *testing.T) {
	t.Parallel()

	const messageCount = 10

	id := make([]byte, 32)
	gsocSvc, cl, signer := newGsocPipeTest(t, id)

	// never read from cl while queuing every message: the first message
	// blocks the single writer goroutine (nothing reads the pipe yet), and
	// the rest pile up behind it in the queue. messageCount is well below
	// api.GsocQueueCapacity, so none of them is evicted.
	payloads := make([][]byte, messageCount)
	for i := range messageCount {
		payloads[i] = []byte{byte(i)}
		ch, _ := cac.New(payloads[i])
		socCh := soc.New(id, ch)
		signedCh, _ := socCh.Sign(signer)
		socCh, _ = soc.FromChunk(signedCh)
		gsocSvc.Handle(socCh)
	}

	// the whole backlog must arrive, in order, once the consumer starts
	// reading again.
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

// TestGsocWebsocketQueueBound verifies that a subscriber which stops reading
// cannot make the server side queue grow without limit: once the queue is full
// the oldest pending messages are evicted, while the most recent ones are still
// delivered, in order.
func TestGsocWebsocketQueueBound(t *testing.T) {
	t.Parallel()

	const messageCount = api.GsocQueueCapacity + 16

	id := make([]byte, 32)
	gsocSvc, cl, signer := newGsocPipeTest(t, id)

	// never read from cl while queuing every message: the first message
	// blocks the single writer goroutine (nothing reads the pipe yet), and
	// the rest pile up behind it until the queue is full and starts dropping
	// its oldest entries.
	for i := range messageCount {
		payload := []byte{byte(i >> 8), byte(i)}
		ch, _ := cac.New(payload)
		socCh := soc.New(id, ch)
		signedCh, _ := socCh.Sign(signer)
		socCh, _ = soc.FromChunk(signedCh)
		gsocSvc.Handle(socCh)
	}

	// the newest message is never evicted, so reading until it arrives
	// terminates. At most one message can have left the queue before the
	// writer blocked on the pipe, which puts the whole backlog the consumer
	// can still see at api.GsocQueueCapacity+1 messages.
	var (
		received int
		last     = -1
	)
	for {
		_, got, err := cl.ReadMessage()
		if err != nil {
			t.Fatalf("message %d: %v", received, err)
		}
		if len(got) != 2 {
			t.Fatalf("message %d: got payload %q, want 2 bytes", received, got)
		}
		received++

		index := int(got[0])<<8 | int(got[1])
		if index <= last {
			t.Fatalf("message %d: got index %d after %d, want increasing order", received, index, last)
		}
		last = index

		if index == messageCount-1 {
			break
		}
	}

	if received > api.GsocQueueCapacity+1 {
		t.Fatalf("got %d messages, want at most %d: the queue is not bounded", received, api.GsocQueueCapacity+1)
	}
}

// TestGsocQueueRelease verifies that the backlog of a subscription whose
// writer is gone is discarded, rather than kept around by a producer that is
// still mid-callback when the connection is torn down.
func TestGsocQueueRelease(t *testing.T) {
	t.Parallel()

	q := api.NewGsocQueue()
	q.Push([]byte("queued before release"))

	q.Release()

	if b, ok := q.Pop(); ok {
		t.Fatalf("got %q after release, want the backlog to be discarded", b)
	}

	q.Push([]byte("queued after release"))
	if b, ok := q.Pop(); ok {
		t.Fatalf("got %q after release, want a late push to be discarded", b)
	}
}

// newGsocPipeTest subscribes to the GSOC address of socID over an in-memory
// net.Pipe instead of a real socket, so that a test fully controls when the
// client reads: writes to a pipe block until the other side reads, which is
// what makes the server side queue observable. It returns the listener to
// publish through, the client end of the subscription and the signer owning
// the subscribed address.
func newGsocPipeTest(t *testing.T, socID []byte) (gsoc.Listener, *websocket.Conn, crypto.Signer) {
	t.Helper()

	var (
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
	chunkAddr, _ := soc.CreateAddress(socID, owner.Bytes())

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

	// Subscribe runs synchronously in the HTTP handler before the connection
	// is handed off to its own goroutine, but Dial returning only means the
	// handshake bytes were exchanged over the pipe — it is not a
	// happens-before guarantee that the server has gone on to call
	// Subscribe. Those are two independent continuations of the same
	// rendezvous, and either goroutine can be scheduled first; a message
	// sent right after Dial can race Subscribe and, if it loses, is dropped
	// before any handler exists to queue it — permanently, not just
	// delayed. Real network I/O (as in TestGsocWebsocketMessageOrdering)
	// involves enough actual syscalls to force scheduler yields that this is
	// a non-issue in practice, but the in-memory pipe used here removes that
	// cushion.
	//
	// Give the server's goroutine time to reach Subscribe before sending
	// anything: this can't be done by retrying a read with a short deadline
	// instead, because gorilla/websocket permanently poisons a *Conn for all
	// further reads once any read — including one that merely times out —
	// returns an error, so a read that might legitimately need a retry
	// cannot be attempted more than once on the same connection.
	time.Sleep(300 * time.Millisecond)

	// Confirm the subscription is actually active (rather than silently
	// trusting the sleep) with a single throwaway round-trip.
	warmup := []byte{0xff}
	{
		ch, _ := cac.New(warmup)
		socCh := soc.New(socID, ch)
		signedCh, _ := socCh.Sign(signer)
		socCh, _ = soc.FromChunk(signedCh)
		gsocSvc.Handle(socCh)
	}
	if err := cl.SetReadDeadline(time.Now().Add(longTimeout)); err != nil {
		t.Fatal(err)
	}
	if _, got, err := cl.ReadMessage(); err != nil {
		t.Fatalf("warmup message: %v", err)
	} else if !bytes.Equal(got, warmup) {
		t.Fatalf("warmup message: got %q, want %q", got, warmup)
	}

	return gsocSvc, cl, signer
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

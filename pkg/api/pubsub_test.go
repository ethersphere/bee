// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pubsub"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/websocket"
	ma "github.com/multiformats/go-multiaddr"
)

func dialWs(t *testing.T, listener, topic string) *websocket.Conn {
	t.Helper()
	u := url.URL{
		Scheme:   "ws",
		Host:     listener,
		Path:     "/pubsub/" + topic,
		RawQuery: "peer=/ip4/127.0.0.1/tcp/9000",
	}
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("websocket dial failed status=%d: %v", resp.StatusCode, err)
		}
		t.Fatalf("websocket dial failed: %v", err)
	}
	return conn
}

// pipeStreamAPI is a bidirectional p2p.Stream backed by an io.Pipe pair.
type pipeStreamAPI struct {
	once sync.Once
	pr   *io.PipeReader
	pw   *io.PipeWriter
}

func newPipeStreamAPI() *pipeStreamAPI {
	pr, pw := io.Pipe()
	return &pipeStreamAPI{pr: pr, pw: pw}
}

func (s *pipeStreamAPI) Read(p []byte) (int, error)   { return s.pr.Read(p) }
func (s *pipeStreamAPI) Write(p []byte) (int, error)  { return s.pw.Write(p) }
func (s *pipeStreamAPI) ResponseHeaders() p2p.Headers { return nil }
func (s *pipeStreamAPI) Headers() p2p.Headers         { return nil }
func (s *pipeStreamAPI) FullClose() error             { return s.Close() }
func (s *pipeStreamAPI) Close() error {
	s.once.Do(func() { s.pr.Close(); s.pw.Close() })
	return nil
}
func (s *pipeStreamAPI) Reset() error { return s.Close() }

// pubsubMockP2P implements pubsub.P2P for testing.
// Only ConnectAllowLight and NewStream do real work; all other methods are stubs.
type pubsubMockP2P struct {
	connectAllowLight func(ctx context.Context, addrs []ma.Multiaddr) (*bzz.Address, error)
	newStream         func(ctx context.Context, address swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error)
}

func (m *pubsubMockP2P) ConnectAllowLight(ctx context.Context, addrs []ma.Multiaddr) (*bzz.Address, error) {
	return m.connectAllowLight(ctx, addrs)
}
func (m *pubsubMockP2P) NewStream(ctx context.Context, address swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	return m.newStream(ctx, address, h, protocol, version, stream)
}
func (m *pubsubMockP2P) AddProtocol(p2p.ProtocolSpec) error { return nil }
func (m *pubsubMockP2P) Connect(context.Context, []ma.Multiaddr) (*bzz.Address, error) {
	return nil, nil
}
func (m *pubsubMockP2P) Disconnect(swarm.Address, string) error               { return nil }
func (m *pubsubMockP2P) Blocklist(swarm.Address, time.Duration, string) error { return nil }
func (m *pubsubMockP2P) NetworkStatus() p2p.NetworkStatus                     { return p2p.NetworkStatusAvailable }
func (m *pubsubMockP2P) Peers() []p2p.Peer                                    { return nil }
func (m *pubsubMockP2P) Blocklisted(swarm.Address) (bool, error)              { return false, nil }
func (m *pubsubMockP2P) BlocklistedPeers() ([]p2p.BlockListedPeer, error)     { return nil, nil }
func (m *pubsubMockP2P) Addresses() ([]ma.Multiaddr, error)                   { return nil, nil }
func (m *pubsubMockP2P) SetPickyNotifier(p2p.PickyNotifier)                   {}
func (m *pubsubMockP2P) Halt()                                                {}

// nopStream is a p2p.Stream that never returns data and ignores all writes.
// It blocks reads until closed, simulating a long-lived idle broker connection.
type nopStream struct {
	once sync.Once
	done chan struct{}
}

func newNopStream() *nopStream { return &nopStream{done: make(chan struct{})} }

func (s *nopStream) Read(p []byte) (int, error) {
	<-s.done
	return 0, io.EOF
}
func (s *nopStream) Write(p []byte) (int, error)  { return len(p), nil }
func (s *nopStream) Close() error                 { s.once.Do(func() { close(s.done) }); return nil }
func (s *nopStream) ResponseHeaders() p2p.Headers { return nil }
func (s *nopStream) Headers() p2p.Headers         { return nil }
func (s *nopStream) FullClose() error             { return s.Close() }
func (s *nopStream) Reset() error                 { return s.Close() }

func newPubsubService(t *testing.T) (*pubsub.Service, *nopStream) {
	t.Helper()
	ns := newNopStream()
	t.Cleanup(func() { _ = ns.Close() })

	mockP2P := &pubsubMockP2P{
		connectAllowLight: func(_ context.Context, _ []ma.Multiaddr) (*bzz.Address, error) {
			return &bzz.Address{Overlay: swarm.NewAddress(make([]byte, 32))}, nil
		},
		newStream: func(_ context.Context, _ swarm.Address, _ p2p.Headers, _, _, _ string) (p2p.Stream, error) {
			return ns, nil
		},
	}
	return pubsub.New(mockP2P, log.Noop, false), ns
}

// newPubsubServiceMultiStream returns a pubsub.Service whose mock P2P creates a
// fresh pipe stream for every NewStream call and collects them for test control.
func newPubsubServiceMultiStream(t *testing.T) (*pubsub.Service, func() []*pipeStreamAPI) {
	t.Helper()
	var mu sync.Mutex
	var streams []*pipeStreamAPI

	mockP2P := &pubsubMockP2P{
		connectAllowLight: func(_ context.Context, _ []ma.Multiaddr) (*bzz.Address, error) {
			return &bzz.Address{Overlay: swarm.NewAddress(make([]byte, 32))}, nil
		},
		newStream: func(_ context.Context, _ swarm.Address, _ p2p.Headers, _, _, _ string) (p2p.Stream, error) {
			ps := newPipeStreamAPI()
			mu.Lock()
			streams = append(streams, ps)
			mu.Unlock()
			return ps, nil
		},
	}
	svc := pubsub.New(mockP2P, log.Noop, false)
	return svc, func() []*pipeStreamAPI {
		mu.Lock()
		defer mu.Unlock()
		return streams
	}
}

func TestPubsubList_NilService(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	resp, err := client.Get("/pubsub/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPubsubList_Empty(t *testing.T) {
	t.Parallel()

	svc := pubsub.New(nil, log.Noop, true)
	client, _, _, _ := newTestServer(t, testServerOptions{PubsubService: svc})

	resp, err := client.Get("/pubsub/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestPubsubWs_MissingPeer(t *testing.T) {
	t.Parallel()

	svc := pubsub.New(nil, log.Noop, false)
	client, _, listener, _ := newTestServer(t, testServerOptions{PubsubService: svc})
	_ = listener

	resp, err := client.Get("/pubsub/testtopic")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing peer, got %d", resp.StatusCode)
	}
}

func TestPubsubWs_InvalidMultiaddr(t *testing.T) {
	t.Parallel()

	svc := pubsub.New(nil, log.Noop, false)
	client, _, _, _ := newTestServer(t, testServerOptions{PubsubService: svc})

	resp, err := client.Get("/pubsub/testtopic?peer=notamultiaddr")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid multiaddr, got %d", resp.StatusCode)
	}
}

func TestPubsubWs_InvalidGsocEthAddress(t *testing.T) {
	t.Parallel()

	svc := pubsub.New(nil, log.Noop, false)
	client, _, _, _ := newTestServer(t, testServerOptions{PubsubService: svc})

	resp, err := client.Get("/pubsub/testtopic?peer=/ip4/127.0.0.1/tcp/9000&gsoc-eth-address=ZZZZ&gsoc-topic=aabb")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid gsoc-eth-address, got %d", resp.StatusCode)
	}
}

func TestPubsubWs_InvalidGsocTopic(t *testing.T) {
	t.Parallel()

	svc := pubsub.New(nil, log.Noop, false)
	client, _, _, _ := newTestServer(t, testServerOptions{PubsubService: svc})

	resp, err := client.Get("/pubsub/testtopic?peer=/ip4/127.0.0.1/tcp/9000&gsoc-eth-address=aabbccddeeff001122334455667788990011223344&gsoc-topic=ZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid gsoc-topic, got %d", resp.StatusCode)
	}
}

func TestPubsubWs_SubscriberConnect(t *testing.T) {
	t.Parallel()

	svc, nopSt := newPubsubService(t)
	_ = nopSt

	_, _, listener, _ := newTestServer(t, testServerOptions{
		PubsubService: svc,
		Logger:        log.Noop,
	})

	u := url.URL{
		Scheme:   "ws",
		Host:     listener,
		Path:     "/pubsub/testtopic",
		RawQuery: "peer=/ip4/127.0.0.1/tcp/9000",
	}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("websocket dial failed with status %d: %v", resp.StatusCode, err)
		}
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close()

	// Connection should be alive; send a close frame and verify clean teardown.
	err = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		t.Fatalf("write close: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, _, err := conn.ReadMessage()
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			break
		}
		if err != nil {
			break
		}
	}
}

func TestPubsubWs_TwoTopicsListAndMessages(t *testing.T) {
	t.Parallel()

	svc, getStreams := newPubsubServiceMultiStream(t)
	client, _, listener, _ := newTestServer(t, testServerOptions{
		PubsubService: svc,
		Logger:        log.Noop,
	})

	// Open two WS connections on different topics.
	conn1 := dialWs(t, listener, "topic-alpha")
	defer conn1.Close()
	conn2 := dialWs(t, listener, "topic-beta")
	defer conn2.Close()

	// Wait until both topics appear as active subscribers.
	err := spinlock.Wait(3*time.Second, func() bool {
		return len(svc.Topics()) == 2
	})
	if err != nil {
		t.Fatalf("timed out waiting for 2 active topics, got %d", len(svc.Topics()))
	}

	// Broker sends a ping (0x01) on each stream to exercise message exchange.
	// A ping is the simplest valid broker message; runMux eats it and keeps running.
	streams := getStreams()
	if len(streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(streams))
	}
	for _, st := range streams {
		if _, err := st.pw.Write([]byte{pubsub.MsgTypePing}); err != nil {
			t.Fatalf("write ping to stream: %v", err)
		}
	}

	// Query the list endpoint — both topics must be present.
	resp, err := client.Get("/pubsub/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Topics []pubsub.TopicInfo `json:"topics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(body.Topics))
	}

	// Close conn1 and verify the topic list shrinks to 1.
	_ = conn1.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	conn1.Close()

	if err := spinlock.Wait(3*time.Second, func() bool {
		return len(svc.Topics()) == 1
	}); err != nil {
		t.Fatalf("timed out waiting for topic count to drop to 1, got %d", len(svc.Topics()))
	}

	// Close conn2 and verify no topics remain.
	_ = conn2.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	conn2.Close()

	if err := spinlock.Wait(3*time.Second, func() bool {
		return len(svc.Topics()) == 0
	}); err != nil {
		t.Fatalf("timed out waiting for topic count to drop to 0, got %d", len(svc.Topics()))
	}
}

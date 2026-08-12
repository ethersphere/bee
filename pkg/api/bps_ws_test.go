// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/soc"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// fakeBpsAttachment is a scripted api.BpsBridge attachment backed by channels.
type fakeBpsAttachment struct {
	spec *pb.CohortSpec
	msgs chan *soc.SOC

	mu     sync.Mutex
	closed bool
	pubC   chan *soc.SOC
}

func newFakeBpsAttachment(spec *pb.CohortSpec) *fakeBpsAttachment {
	return &fakeBpsAttachment{
		spec: spec,
		msgs: make(chan *soc.SOC, 4),
		pubC: make(chan *soc.SOC, 4),
	}
}

func (a *fakeBpsAttachment) Spec() *pb.CohortSpec      { return a.spec }
func (a *fakeBpsAttachment) Messages() <-chan *soc.SOC { return a.msgs }
func (a *fakeBpsAttachment) Publish(_ context.Context, s *soc.SOC) error {
	a.pubC <- s
	return nil
}

func (a *fakeBpsAttachment) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	return nil
}

func (a *fakeBpsAttachment) isClosed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

// fakeBpsBridge records the options of the last attach and answers with a
// scripted attachment or a scripted error.
type fakeBpsBridge struct {
	att    *fakeBpsAttachment
	err    error
	status []bps.TopicStatus

	mu   sync.Mutex
	opts *bps.AttachOptions
}

func (b *fakeBpsBridge) Attach(_ context.Context, o bps.AttachOptions) (bps.Attachment, error) {
	b.mu.Lock()
	cp := o
	b.opts = &cp
	b.mu.Unlock()
	if b.err != nil {
		return nil, b.err
	}
	return b.att, nil
}

func (b *fakeBpsBridge) Status() []bps.TopicStatus { return b.status }

func (b *fakeBpsBridge) attached() *bps.AttachOptions {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.opts
}

func dialBpsWs(t *testing.T, addr, path string, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	u := url.URL{Scheme: "ws", Host: addr, Path: path}
	if p, q, ok := strings.Cut(path, "?"); ok {
		u.Path, u.RawQuery = p, q
	}
	return websocket.DefaultDialer.Dial(u.String(), header)
}

func TestBpsWsPublishSubscribe(t *testing.T) {
	t.Parallel()

	signerA, ownerA := bpstesting.NewSigner(t)
	_, ownerB := bpstesting.NewSigner(t)
	_, ownerC := bpstesting.NewSigner(t)

	topic := swarm.RandAddress(t)

	att := newFakeBpsAttachment(&pb.CohortSpec{
		Topic:      topic.Bytes(),
		Binding:    pb.TopicBinding_ANCHOR,
		Publishers: pb.PublisherRegime_EXPLICIT_LIST,
		Admin:      ownerA,
	})
	bridge := &fakeBpsBridge{att: att}

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    bridge,
	})

	q := url.Values{}
	q.Set("peer", "/ip4/127.0.0.1/tcp/1634")
	q.Set("binding", "anchor")
	q.Set("publishers", "list")
	q.Set("closed", "true")
	q.Set("admin", hex.EncodeToString(ownerA))
	q.Set("publisher-list", hex.EncodeToString(ownerB)+","+hex.EncodeToString(ownerC))
	q.Set("owner", hex.EncodeToString(ownerA))

	u := url.URL{Scheme: "ws", Host: addr, Path: "/pubsub/" + topic.String(), RawQuery: q.Encode()}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	o := bridge.attached()
	if o == nil {
		t.Fatal("bridge was not attached")
	}
	if o.Spec == nil {
		t.Fatal("expected an assembled cohort spec")
	}
	if !bytes.Equal(o.Spec.GetTopic(), topic.Bytes()) {
		t.Fatalf("spec topic: got %x want %x", o.Spec.GetTopic(), topic.Bytes())
	}
	if o.Spec.GetBinding() != pb.TopicBinding_ANCHOR {
		t.Fatalf("spec binding: got %v", o.Spec.GetBinding())
	}
	if o.Spec.GetPublishers() != pb.PublisherRegime_EXPLICIT_LIST {
		t.Fatalf("spec publishers: got %v", o.Spec.GetPublishers())
	}
	if !o.Spec.GetClosed() {
		t.Fatal("spec closed: got false want true")
	}
	if !bytes.Equal(o.Spec.GetAdmin(), ownerA) {
		t.Fatalf("spec admin: got %x want %x", o.Spec.GetAdmin(), ownerA)
	}
	if len(o.Spec.GetPublisherList()) != 2 {
		t.Fatalf("publisher list: got %d entries want 2", len(o.Spec.GetPublisherList()))
	}
	if !bytes.Equal(o.Owner, ownerA) {
		t.Fatalf("owner: got %x want %x", o.Owner, ownerA)
	}
	if o.Peer == nil {
		t.Fatal("peer multiaddr not passed through")
	}

	// publish one anchor frame
	payload := []byte("published payload")
	sc := bpstesting.AnchorSOC(t, signerA, topic.Bytes(), payload)
	frame := append(append([]byte{}, sc.Signature()...), sc.WrappedChunk().Data()...)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-att.pubC:
		if !bytes.Equal(got.ID(), topic.Bytes()) {
			t.Fatalf("published soc id: got %x want %x", got.ID(), topic.Bytes())
		}
		if !bytes.Equal(got.WrappedChunk().Data()[swarm.SpanSize:], payload) {
			t.Fatal("published payload mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for publish")
	}

	// receive one message
	inPayload := []byte("inbound payload")
	in := bpstesting.AnchorSOC(t, signerA, topic.Bytes(), inPayload)
	att.msgs <- in

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mt, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("message type: got %d want binary", mt)
	}
	if !bytes.Equal(data, inPayload) {
		t.Fatalf("got %q want %q", data, inPayload)
	}
}

func TestBpsWsSubscribeOnly(t *testing.T) {
	t.Parallel()

	topic := swarm.RandAddress(t)
	att := newFakeBpsAttachment(nil)
	bridge := &fakeBpsBridge{att: att}

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    bridge,
	})

	conn, _, err := dialBpsWs(t, addr, "/pubsub/"+topic.String()+"?peer=/ip4/127.0.0.1/tcp/1634", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	o := bridge.attached()
	if o == nil {
		t.Fatal("bridge was not attached")
	}
	if o.Spec != nil {
		t.Fatalf("spec: got %v want nil", o.Spec)
	}
	if o.Owner != nil {
		t.Fatalf("owner: got %x want nil", o.Owner)
	}
	if !o.Topic.Equal(topic) {
		t.Fatalf("topic: got %s want %s", o.Topic, topic)
	}
}

func TestBpsWsRefusalMapping(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"full", &bps.RefusalError{Status: pb.Status_FULL}, http.StatusServiceUnavailable},
		{"unknown topic", &bps.RefusalError{Status: pb.Status_UNKNOWN_TOPIC}, http.StatusNotFound},
		{"rejected", &bps.RefusalError{Status: pb.Status_REJECTED}, http.StatusForbidden},
		{"no peer", bps.ErrNoPeer, http.StatusBadRequest},
		{"spec mismatch", bps.ErrSpecMismatch, http.StatusConflict},
		{"not publisher", bps.ErrNotPublisher, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			topic := swarm.RandAddress(t)
			bridge := &fakeBpsBridge{err: tc.err}

			_, _, addr, _ := newTestServer(t, testServerOptions{
				Storer: mockstorer.New(),
				Bps:    bridge,
			})

			_, resp, err := dialBpsWs(t, addr, "/pubsub/"+topic.String()+"?peer=/ip4/127.0.0.1/tcp/1634", nil)
			if err == nil {
				t.Fatal("expected the handshake to be refused")
			}
			if resp == nil {
				t.Fatalf("no http response: %v", err)
			}
			if resp.StatusCode != tc.status {
				t.Fatalf("status: got %d want %d", resp.StatusCode, tc.status)
			}
		})
	}
}

func TestBpsWsBadParams(t *testing.T) {
	t.Parallel()

	topic := swarm.RandAddress(t)

	for _, tc := range []struct {
		name string
		path string
	}{
		{"missing peer", "/pubsub/" + topic.String()},
		{"invalid peer", "/pubsub/" + topic.String() + "?peer=notamultiaddr"},
		{"invalid binding", "/pubsub/" + topic.String() + "?peer=/ip4/127.0.0.1/tcp/1634&binding=bogus"},
		{"incomplete spec", "/pubsub/" + topic.String() + "?peer=/ip4/127.0.0.1/tcp/1634&binding=anchor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bridge := &fakeBpsBridge{att: newFakeBpsAttachment(nil)}
			_, _, addr, _ := newTestServer(t, testServerOptions{
				Storer: mockstorer.New(),
				Bps:    bridge,
			})

			_, resp, err := dialBpsWs(t, addr, tc.path, nil)
			if err == nil {
				t.Fatal("expected the handshake to be refused")
			}
			if resp == nil {
				t.Fatalf("no http response: %v", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status: got %d want %d", resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

func TestBpsNotEnabled(t *testing.T) {
	t.Parallel()

	client, _, addr, _ := newTestServer(t, testServerOptions{Storer: mockstorer.New()})

	t.Run("topics", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/pubsub", http.StatusNotImplemented,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "pubsub not enabled",
				Code:    http.StatusNotImplemented,
			}),
		)
	})

	t.Run("websocket", func(t *testing.T) {
		topic := swarm.RandAddress(t)

		_, resp, err := dialBpsWs(t, addr, "/pubsub/"+topic.String()+"?peer=/ip4/127.0.0.1/tcp/1634", nil)
		if err == nil {
			t.Fatal("expected the handshake to be refused")
		}
		if resp == nil {
			t.Fatalf("no http response: %v", err)
		}
		if resp.StatusCode != http.StatusNotImplemented {
			t.Fatalf("status: got %d want %d", resp.StatusCode, http.StatusNotImplemented)
		}
	})
}

func TestBpsWsBadHeaders(t *testing.T) {
	t.Parallel()

	topic := swarm.RandAddress(t)

	for _, tc := range []struct {
		name   string
		header http.Header
	}{
		{"zero keep alive", http.Header{api.SwarmKeepAliveHeader: {"0"}}},
		{"negative keep alive", http.Header{api.SwarmKeepAliveHeader: {"-1"}}},
		{"non numeric keep alive", http.Header{api.SwarmKeepAliveHeader: {"soon"}}},
		{"unknown soc field", http.Header{api.SwarmSocFieldsHeader: {"identifier,bogus"}}},
		{"non boolean cache wrapped chunk", http.Header{api.SwarmCacheWrappedChunkHeader: {"maybe"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bridge := &fakeBpsBridge{att: newFakeBpsAttachment(nil)}
			_, _, addr, _ := newTestServer(t, testServerOptions{
				Storer: mockstorer.New(),
				Bps:    bridge,
			})

			_, resp, err := dialBpsWs(t, addr, "/pubsub/"+topic.String()+"?peer=/ip4/127.0.0.1/tcp/1634", tc.header)
			if err == nil {
				t.Fatal("expected the handshake to be refused")
			}
			if resp == nil {
				t.Fatalf("no http response: %v", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status: got %d want %d", resp.StatusCode, http.StatusBadRequest)
			}
			if bridge.attached() != nil {
				t.Fatal("bridge was attached despite a bad header")
			}
		})
	}
}

func TestBpsWsSocFieldsHeader(t *testing.T) {
	t.Parallel()

	signer, _ := bpstesting.NewSigner(t)
	topic := swarm.RandAddress(t)
	att := newFakeBpsAttachment(nil)
	bridge := &fakeBpsBridge{att: att}

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    bridge,
	})

	header := http.Header{
		api.SwarmSocFieldsHeader: {"identifier,payload"},
		api.SwarmKeepAliveHeader: {"120"},
	}
	conn, _, err := dialBpsWs(t, addr, "/pubsub/"+topic.String()+"?peer=/ip4/127.0.0.1/tcp/1634", header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	payload := []byte("json framed payload")
	att.msgs <- bpstesting.AnchorSOC(t, signer, topic.Bytes(), payload)

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("message type: got %d want text", msgType)
	}

	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("got %d keys want exactly 2: %+v", len(m), m)
	}
	if m["identifier"] != topic.String() {
		t.Fatalf("identifier: got %q want %q", m["identifier"], topic.String())
	}
	if m["payload"] != hex.EncodeToString(payload) {
		t.Fatalf("payload: got %q want %q", m["payload"], hex.EncodeToString(payload))
	}
}

func TestBpsWsCacheWrappedChunk(t *testing.T) {
	t.Parallel()

	signer, _ := bpstesting.NewSigner(t)
	topic := swarm.RandAddress(t)
	att := newFakeBpsAttachment(nil)
	bridge := &fakeBpsBridge{att: att}

	storer := mockstorer.New()
	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storer,
		Bps:    bridge,
	})

	header := http.Header{api.SwarmCacheWrappedChunkHeader: {"true"}}
	conn, _, err := dialBpsWs(t, addr, "/pubsub/"+topic.String()+"?peer=/ip4/127.0.0.1/tcp/1634", header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	payload := []byte("cached payload")
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), payload)
	wrapped := sc.WrappedChunk()
	att.msgs <- sc

	// the message is still delivered
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, data, err := conn.ReadMessage(); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(data, payload) {
		t.Fatalf("got %q want %q", data, payload)
	}

	// the wrapped chunk reached the cache: the mock storer's Cache putter and
	// its ChunkStore are the same store, so the Put is observable here.
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		got, err := storer.ChunkStore().Get(ctx, wrapped.Address())
		if err == nil {
			if !bytes.Equal(got.Data(), wrapped.Data()) {
				t.Fatal("cached chunk data mismatch")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("wrapped chunk was not cached")
}

func TestBpsWsMnemonicTopic(t *testing.T) {
	t.Parallel()

	att := newFakeBpsAttachment(nil)
	bridge := &fakeBpsBridge{att: att}

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    bridge,
	})

	conn, _, err := dialBpsWs(t, addr, "/pubsub/my-topic?peer=/ip4/127.0.0.1/tcp/1634", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	want, err := api.BpsResolveTopic("my-topic")
	if err != nil {
		t.Fatal(err)
	}
	o := bridge.attached()
	if o == nil {
		t.Fatal("bridge was not attached")
	}
	if !o.Topic.Equal(want) {
		t.Fatalf("topic: got %s want %s", o.Topic, want)
	}
}

func TestBpsWsSessionEnd(t *testing.T) {
	t.Parallel()

	topic := swarm.RandAddress(t)
	att := newFakeBpsAttachment(nil)
	bridge := &fakeBpsBridge{att: att}

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    bridge,
	})

	conn, _, err := dialBpsWs(t, addr, "/pubsub/"+topic.String()+"?peer=/ip4/127.0.0.1/tcp/1634", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// the broker ends the session
	close(att.msgs)

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected the connection to be closed by the node")
	}

	for i := 0; i < 100; i++ {
		if att.isClosed() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("attachment was not closed")
}

func TestBpsTopics(t *testing.T) {
	t.Parallel()

	_, ownerA := bpstesting.NewSigner(t)
	_, ownerB := bpstesting.NewSigner(t)
	topic := swarm.RandAddress(t)

	bridge := &fakeBpsBridge{
		status: []bps.TopicStatus{
			{
				Topic: topic,
				Spec: &pb.CohortSpec{
					Topic:         topic.Bytes(),
					Binding:       pb.TopicBinding_FEED_TOPIC,
					Publishers:    pb.PublisherRegime_EXPLICIT_LIST,
					Admin:         ownerA,
					PublisherList: [][]byte{ownerB},
					Closed:        true,
				},
				Broker: true,
				Peers:  3,
			},
		},
	}

	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    bridge,
	})

	jsonhttptest.Request(t, client, http.MethodGet, "/pubsub", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse([]api.BpsTopicResponse{
			{
				Topic: topic.String(),
				Role:  "broker",
				Peers: 3,
				Cohort: &api.BpsCohortResponse{
					Binding:       "feed",
					Publishers:    "list",
					Admin:         hex.EncodeToString(ownerA),
					PublisherList: []string{hex.EncodeToString(ownerB)},
					Closed:        true,
					History:       false,
				},
			},
		}),
	)
}

func TestBpsTopicsEmpty(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    &fakeBpsBridge{},
	})

	jsonhttptest.Request(t, client, http.MethodGet, "/pubsub", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse([]api.BpsTopicResponse{}),
	)
}

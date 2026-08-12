// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/soc"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The tests in this file are the SWIP-60 §5 conformance scenarios end to end:
// a real broker service, real client services and real bridges over a
// streamtest network, driven only through the pubsub WebSocket API. Nothing
// here is faked except the peer transport and the Connecter that resolves a
// broker underlay to its overlay.

// e2eUnderlay is the broker underlay every attach names. It is never dialed:
// e2eConnecter answers it with the broker's overlay.
const e2eUnderlay = "/ip4/127.0.0.1/tcp/1634"

// e2eConnecter resolves every underlay to the one broker of the test network.
type e2eConnecter struct {
	addr *bzz.Address
}

func (c *e2eConnecter) Connect(context.Context, []ma.Multiaddr) (*bzz.Address, error) {
	return c.addr, nil
}

// e2eNode is one Bee node of the test network: a client bps service on its own
// recorder, a bridge over it, and an API server serving the pubsub endpoint.
type e2eNode struct {
	addr     string
	recorder *streamtest.Recorder
}

// newE2EBroker returns a running broker service and the overlay it is reached
// at. The broker holds no streamer of its own — it only ever answers.
func newE2EBroker(t *testing.T, o bps.Options) (*bps.Service, swarm.Address) {
	t.Helper()

	broker := bps.New(nil, log.Noop, o)
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return broker, swarm.MustParseHexAddress("ca11ab1e")
}

// newE2ENode wires one node onto the broker. Every node gets a distinct base
// overlay, which is the address the broker sees the stream arrive from: the
// cohort keys its retained streams by peer, so two nodes sharing one base
// address would not be two peers.
func newE2ENode(t *testing.T, broker *bps.Service, brokerAddr, base swarm.Address) *e2eNode {
	t.Helper()

	recorder := streamtest.New(
		streamtest.WithProtocols(broker.Protocol()),
		streamtest.WithBaseAddr(base),
	)

	client := bps.New(recorder, log.Noop, bps.Options{})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	})

	underlay, err := ma.NewMultiaddr(e2eUnderlay)
	if err != nil {
		t.Fatal(err)
	}
	bridge := bps.NewBridge(client, &e2eConnecter{addr: &bzz.Address{
		Underlays: []ma.Multiaddr{underlay},
		Overlay:   brokerAddr,
	}}, log.Noop)
	t.Cleanup(func() {
		if err := bridge.Close(); err != nil {
			t.Fatal(err)
		}
	})

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Bps:    bridge,
	})

	return &e2eNode{addr: addr, recorder: recorder}
}

// anchorFrame assembles an ANCHOR publish frame: sig(65) ‖ span(8) ‖ payload.
func anchorFrame(sc *soc.SOC) []byte {
	frame := make([]byte, 0, swarm.SocSignatureSize+len(sc.WrappedChunk().Data()))
	frame = append(frame, sc.Signature()...)
	return append(frame, sc.WrappedChunk().Data()...)
}

// feedFrame assembles a FEED_TOPIC publish frame: index(8 BE) ‖ sig ‖ span ‖
// payload. The index is not on the wire between nodes; it is how the
// publisher's own node re-derives the SOC id.
func feedFrame(index uint64, sc *soc.SOC) []byte {
	return append(binary.BigEndian.AppendUint64(nil, index), anchorFrame(sc)...)
}

// expectPayload reads binary frames until one carries want, or the deadline
// passes. A publisher's own socket also sees what it published, so a test
// looking for one payload has to be willing to skip others.
func expectPayload(t *testing.T, conn *websocket.Conn, want []byte) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for payload %q: %v", want, err)
		}
		if mt == websocket.BinaryMessage && bytes.Equal(data, want) {
			return
		}
	}
}

// readSocFields reads one JSON text frame of selected SOC fields.
func readSocFields(t *testing.T, conn *websocket.Conn) map[string]string {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	mt, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("message type: got %d want text", mt)
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestConformanceJamCohort(t *testing.T) {
	t.Parallel()

	broker, brokerAddr := newE2EBroker(t, bps.Options{})
	nodeA := newE2ENode(t, broker, brokerAddr, swarm.MustParseHexAddress("aa01"))
	nodeB := newE2ENode(t, broker, brokerAddr, swarm.MustParseHexAddress("bb02"))
	nodeE := newE2ENode(t, broker, brokerAddr, swarm.MustParseHexAddress("ee05"))

	signerA, ownerA := bpstesting.NewSigner(t)
	signerB, ownerB := bpstesting.NewSigner(t)
	_, ownerC := bpstesting.NewSigner(t)
	_, ownerD := bpstesting.NewSigner(t)

	// The path segment is a mnemonic: the node hashes it into the topic, and
	// the dApp holding the keys has to sign the same id.
	const mnemonic = "jam-tuesday"
	topic, err := crypto.LegacyKeccak256([]byte(mnemonic))
	if err != nil {
		t.Fatal(err)
	}

	// A opens the closed, list-publisher cohort as its admin.
	qa := url.Values{}
	qa.Set("peer", e2eUnderlay)
	qa.Set("binding", "anchor")
	qa.Set("publishers", "list")
	qa.Set("closed", "true")
	qa.Set("admin", hex.EncodeToString(ownerA))
	qa.Set("publisher-list", hex.EncodeToString(ownerB)+","+hex.EncodeToString(ownerC)+","+hex.EncodeToString(ownerD))
	qa.Set("owner", hex.EncodeToString(ownerA))

	connA, _, err := dialBpsWs(t, nodeA.addr, "/pubsub/"+mnemonic+"?"+qa.Encode(), nil)
	if err != nil {
		t.Fatalf("admin dial: %v", err)
	}
	defer connA.Close()

	// B joins the live cohort from another node, naming only its owner: no
	// cohort parameters means Subscribe, and the owner makes it a publisher.
	qb := url.Values{}
	qb.Set("peer", e2eUnderlay)
	qb.Set("owner", hex.EncodeToString(ownerB))

	connB, _, err := dialBpsWs(t, nodeB.addr, "/pubsub/"+mnemonic+"?"+qb.Encode(), nil)
	if err != nil {
		t.Fatalf("member dial: %v", err)
	}
	defer connB.Close()

	// A publishes, B receives.
	fromA := []byte("scones at three")
	scA := bpstesting.AnchorSOC(t, signerA, topic, fromA)
	if err := connA.WriteMessage(websocket.BinaryMessage, anchorFrame(scA)); err != nil {
		t.Fatal(err)
	}
	expectPayload(t, connB, fromA)

	// B publishes, A receives.
	fromB := []byte("bring the clotted cream")
	scB := bpstesting.AnchorSOC(t, signerB, topic, fromB)
	if err := connB.WriteMessage(websocket.BinaryMessage, anchorFrame(scB)); err != nil {
		t.Fatal(err)
	}
	expectPayload(t, connA, fromB)

	// A fifth client on a node that has no session for the topic, presenting
	// no owner, is refused admission to the closed cohort.
	qe := url.Values{}
	qe.Set("peer", e2eUnderlay)

	_, resp, err := dialBpsWs(t, nodeE.addr, "/pubsub/"+mnemonic+"?"+qe.Encode(), nil)
	if err == nil {
		t.Fatal("outsider was admitted to a closed cohort")
	}
	if resp == nil {
		t.Fatalf("no http response: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider status: got %d want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestConformanceLiveStream(t *testing.T) {
	t.Parallel()

	broker, brokerAddr := newE2EBroker(t, bps.Options{})
	pubNode := newE2ENode(t, broker, brokerAddr, swarm.MustParseHexAddress("aa11"))
	subNode := newE2ENode(t, broker, brokerAddr, swarm.MustParseHexAddress("bb12"))

	signerP, ownerP := bpstesting.NewSigner(t)
	topic := swarm.RandAddress(t)

	qp := url.Values{}
	qp.Set("peer", e2eUnderlay)
	qp.Set("binding", "feed")
	qp.Set("publishers", "single")
	qp.Set("admin", hex.EncodeToString(ownerP))
	qp.Set("owner", hex.EncodeToString(ownerP))

	pubConn, _, err := dialBpsWs(t, pubNode.addr, "/pubsub/"+topic.String()+"?"+qp.Encode(), nil)
	if err != nil {
		t.Fatalf("publisher dial: %v", err)
	}
	defer pubConn.Close()

	qs := url.Values{}
	qs.Set("peer", e2eUnderlay)

	header := http.Header{api.SwarmSocFieldsHeader: {"identifier,payload"}}
	subConn, _, err := dialBpsWs(t, subNode.addr, "/pubsub/"+topic.String()+"?"+qs.Encode(), header)
	if err != nil {
		t.Fatalf("subscriber dial: %v", err)
	}
	defer subConn.Close()

	payloads := [][]byte{[]byte("frame zero"), []byte("frame one")}
	frames := make([][]byte, len(payloads))
	for i, p := range payloads {
		sc := bpstesting.FeedSOC(t, signerP, topic.Bytes(), uint64(i), p)
		frames[i] = feedFrame(uint64(i), sc)
		if err := pubConn.WriteMessage(websocket.BinaryMessage, frames[i]); err != nil {
			t.Fatal(err)
		}
	}

	for i, p := range payloads {
		got := readSocFields(t, subConn)

		id, err := bps.FeedID(topic.Bytes(), uint64(i))
		if err != nil {
			t.Fatal(err)
		}
		if got["identifier"] != hex.EncodeToString(id) {
			t.Fatalf("index %d identifier: got %s want %s", i, got["identifier"], hex.EncodeToString(id))
		}
		if got["payload"] != hex.EncodeToString(p) {
			t.Fatalf("index %d payload: got %s want %s", i, got["payload"], hex.EncodeToString(p))
		}
	}

	// Republishing index 0 verbatim is deduplicated by the broker, so no third
	// message reaches the subscriber. A fresh index 2 published straight after
	// it is the drain marker: ordering is preserved end to end — the broker
	// reads one publisher's stream serially and enqueues fan-out in order — so
	// if the duplicate had been rebroadcast it would be sitting ahead of index
	// 2 in the subscriber's queue. Asserting that the *next* message read is
	// index 2 therefore cannot pass merely because the network was slow, which
	// a bare read deadline could.
	if err := pubConn.WriteMessage(websocket.BinaryMessage, frames[0]); err != nil {
		t.Fatal(err)
	}
	marker := bpstesting.FeedSOC(t, signerP, topic.Bytes(), 2, []byte("frame two"))
	if err := pubConn.WriteMessage(websocket.BinaryMessage, feedFrame(2, marker)); err != nil {
		t.Fatal(err)
	}

	got := readSocFields(t, subConn)
	markerID, err := bps.FeedID(topic.Bytes(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if got["identifier"] != hex.EncodeToString(markerID) {
		t.Fatalf("message after the duplicate: got identifier %s want %s (the duplicate was rebroadcast)",
			got["identifier"], hex.EncodeToString(markerID))
	}

	// Secondary check: nothing at all trails the marker.
	if err := subConn.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, data, err := subConn.ReadMessage(); err == nil {
		t.Fatalf("unexpected message after the drain marker: %q", data)
	}
}

func TestConformanceFull(t *testing.T) {
	t.Parallel()

	broker, brokerAddr := newE2EBroker(t, bps.Options{Capacity: 1})
	nodeA := newE2ENode(t, broker, brokerAddr, swarm.MustParseHexAddress("aa21"))
	nodeB := newE2ENode(t, broker, brokerAddr, swarm.MustParseHexAddress("bb22"))

	_, ownerP := bpstesting.NewSigner(t)
	topic := swarm.RandAddress(t)

	qa := url.Values{}
	qa.Set("peer", e2eUnderlay)
	qa.Set("binding", "anchor")
	qa.Set("publishers", "single")
	qa.Set("admin", hex.EncodeToString(ownerP))
	qa.Set("owner", hex.EncodeToString(ownerP))

	connA, _, err := dialBpsWs(t, nodeA.addr, "/pubsub/"+topic.String()+"?"+qa.Encode(), nil)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer connA.Close()

	// The single stream slot is taken, so the second node's Subscribe is
	// refused with FULL, which the API answers as 503.
	qb := url.Values{}
	qb.Set("peer", e2eUnderlay)

	_, resp, err := dialBpsWs(t, nodeB.addr, "/pubsub/"+topic.String()+"?"+qb.Encode(), nil)
	if err == nil {
		t.Fatal("second session was admitted to a full cohort")
	}
	if resp == nil {
		t.Fatalf("no http response: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	// At the wire level the refused stream carried exactly one Hello and one
	// Ack: a refusal is answered and reset, never referred or half served.
	// Node B has its own recorder, so only its own refused stream is listed.
	records, err := nodeB.recorder.Records(brokerAddr, bps.ProtocolName, bps.ProtocolVersion, bps.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("streams to the broker: got %d want 1", len(records))
	}

	in := protobuf.NewReader(bytes.NewReader(records[0].In()))
	var hello pb.Hello
	if err := in.ReadMsg(&hello); err != nil {
		t.Fatal(err)
	}
	if err := in.ReadMsg(&hello); !errors.Is(err, io.EOF) {
		t.Fatalf("client wrote more than one Hello: %v", err)
	}

	out := protobuf.NewReader(bytes.NewReader(records[0].Out()))
	var ack pb.Ack
	if err := out.ReadMsg(&ack); err != nil {
		t.Fatal(err)
	}
	if ack.Status != pb.Status_FULL {
		t.Fatalf("ack status: got %s want FULL", ack.Status)
	}
	if err := out.ReadMsg(&ack); !errors.Is(err, io.EOF) {
		t.Fatalf("broker wrote more than the refusal Ack: %v", err)
	}
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/websocket"
)

func TestBpsFrameAnchorRoundTrip(t *testing.T) {
	t.Parallel()

	signer, owner := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	payload := []byte("anchor payload")

	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), payload)
	wantAddr, err := sc.Address()
	if err != nil {
		t.Fatal(err)
	}

	frame := append(append([]byte{}, sc.Signature()...), sc.WrappedChunk().Data()...)

	got, err := api.ParsePublishFrame(pb.TopicBinding_ANCHOR, topic, owner, frame)
	if err != nil {
		t.Fatal(err)
	}
	gotAddr, err := got.Address()
	if err != nil {
		t.Fatal(err)
	}
	if !gotAddr.Equal(wantAddr) {
		t.Fatalf("address mismatch: got %s want %s", gotAddr, wantAddr)
	}
}

func TestBpsFrameFeedTopicRoundTrip(t *testing.T) {
	t.Parallel()

	signer, owner := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	payload := []byte("feed payload")
	const index = uint64(7)

	sc := bpstesting.FeedSOC(t, signer, topic.Bytes(), index, payload)
	wantAddr, err := sc.Address()
	if err != nil {
		t.Fatal(err)
	}

	indexBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(indexBytes, index)
	frame := append(indexBytes, sc.Signature()...)
	frame = append(frame, sc.WrappedChunk().Data()...)

	got, err := api.ParsePublishFrame(pb.TopicBinding_FEED_TOPIC, topic, owner, frame)
	if err != nil {
		t.Fatal(err)
	}
	gotAddr, err := got.Address()
	if err != nil {
		t.Fatal(err)
	}
	if !gotAddr.Equal(wantAddr) {
		t.Fatalf("address mismatch: got %s want %s", gotAddr, wantAddr)
	}
}

func TestBpsFrameTruncatedRejected(t *testing.T) {
	t.Parallel()

	signer, owner := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), []byte("data"))

	frame := append(append([]byte{}, sc.Signature()...), sc.WrappedChunk().Data()...)

	// Truncate down to something shorter than sig+span to be unambiguous.
	short := frame[:swarm.SocSignatureSize+swarm.SpanSize-1]

	if _, err := api.ParsePublishFrame(pb.TopicBinding_ANCHOR, topic, owner, short); err == nil {
		t.Fatal("expected error for truncated frame")
	}
}

func TestBpsFrameBadSignatureRejected(t *testing.T) {
	t.Parallel()

	signer, owner := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), []byte("data"))

	frame := append(append([]byte{}, sc.Signature()...), sc.WrappedChunk().Data()...)
	// Corrupt one byte of the signature.
	frame[0] ^= 0xff

	if _, err := api.ParsePublishFrame(pb.TopicBinding_ANCHOR, topic, owner, frame); err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestBpsFrameOversizedPayloadRejected(t *testing.T) {
	t.Parallel()

	_, owner := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())

	// A well-formed frame shape (sig + span + payload) but with a payload
	// larger than swarm.ChunkSize; the signature and span contents don't
	// need to be valid since the size check must reject it first.
	sig := make([]byte, swarm.SocSignatureSize)
	span := make([]byte, swarm.SpanSize)
	payload := make([]byte, swarm.ChunkSize+1)
	frame := append(append(sig, span...), payload...)

	if _, err := api.ParsePublishFrame(pb.TopicBinding_ANCHOR, topic, owner, frame); err == nil {
		t.Fatal("expected error for oversized payload")
	}
}

func TestBpsFrameUnsupportedBindingRejected(t *testing.T) {
	t.Parallel()

	signer, owner := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), []byte("data"))

	frame := append(append([]byte{}, sc.Signature()...), sc.WrappedChunk().Data()...)

	if _, err := api.ParsePublishFrame(pb.TopicBinding_OWNER, topic, owner, frame); err == nil {
		t.Fatal("expected error for unsupported binding")
	}
}

func TestBpsSocFieldsDefault(t *testing.T) {
	t.Parallel()

	f, err := api.ParseSocFields("")
	if err != nil {
		t.Fatal(err)
	}

	signer, _ := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	payload := []byte("default field selection")
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), payload)

	msgType, data, err := api.SerializeSoc(f, sc)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("got msgType %d want BinaryMessage (default should be payload-only)", msgType)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("got %x want %x", data, payload)
	}
}

func TestBpsSocFieldsParsesKnown(t *testing.T) {
	t.Parallel()

	f, err := api.ParseSocFields(" identifier , payload ")
	if err != nil {
		t.Fatal(err)
	}

	signer, _ := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), []byte("data"))

	msgType, data, err := api.SerializeSoc(f, sc)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("got msgType %d want TextMessage", msgType)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("got %d keys, want exactly 2: %+v", len(m), m)
	}
}

func TestBpsSocFieldsUnknownRejected(t *testing.T) {
	t.Parallel()

	if _, err := api.ParseSocFields("bogus"); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestBpsSerializePayloadOnlyBinary(t *testing.T) {
	t.Parallel()

	signer, _ := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	payload := []byte("hello world")
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), payload)

	f, err := api.ParseSocFields("")
	if err != nil {
		t.Fatal(err)
	}
	msgType, data, err := api.SerializeSoc(f, sc)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("got msgType %d want BinaryMessage", msgType)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("got %x want %x", data, payload)
	}
}

func TestBpsSerializeIdentifierPayloadJSON(t *testing.T) {
	t.Parallel()

	signer, _ := bpstesting.NewSigner(t)
	topic := swarm.NewAddress(swarm.RandAddress(t).Bytes())
	payload := []byte("hello world")
	sc := bpstesting.AnchorSOC(t, signer, topic.Bytes(), payload)

	f, err := api.ParseSocFields("identifier,payload")
	if err != nil {
		t.Fatal(err)
	}
	msgType, data, err := api.SerializeSoc(f, sc)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("got msgType %d want TextMessage", msgType)
	}

	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("got %d keys, want exactly 2: %+v", len(m), m)
	}
	if _, ok := m["identifier"]; !ok {
		t.Fatal("missing identifier key")
	}
	if _, ok := m["payload"]; !ok {
		t.Fatal("missing payload key")
	}
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Testing out Mode 1 (GSoC Ephemeral) implementation

package pubsub_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pubsub"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// readerStream is a p2p.Stream whose Read side is backed by a bytes.Reader.
// Write calls succeed but are discarded. Close/Reset are no-ops.
type readerStream struct {
	io.Reader
	headers p2p.Headers
}

func (r *readerStream) Write(p []byte) (int, error)  { return len(p), nil }
func (r *readerStream) Close() error                 { return nil }
func (r *readerStream) ResponseHeaders() p2p.Headers { return nil }
func (r *readerStream) Headers() p2p.Headers         { return r.headers }
func (r *readerStream) FullClose() error             { return nil }
func (r *readerStream) Reset() error                 { return nil }

func newReaderStream(data []byte, headers p2p.Headers) *readerStream {
	return &readerStream{Reader: bytes.NewReader(data), headers: headers}
}

// socTestCtx holds key material for building valid SOC publisher messages.
type socTestCtx struct {
	signer    crypto.Signer
	owner     []byte
	gsocID    []byte
	topicAddr [32]byte
}

func newSocTestCtx(t *testing.T) *socTestCtx {
	t.Helper()

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	ownerAddr, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	gsocID := make([]byte, swarm.HashSize)

	topicAddress, err := soc.CreateAddress(gsocID, ownerAddr.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	var topicArr [32]byte
	copy(topicArr[:], topicAddress.Bytes())

	return &socTestCtx{
		signer:    signer,
		owner:     ownerAddr.Bytes(),
		gsocID:    gsocID,
		topicAddr: topicArr,
	}
}

// buildPublisherMsg returns a valid pubsub publisher wire message: sig(65B)+span(8B)+payload.
func buildPublisherMsg(t *testing.T, tc *socTestCtx, payload []byte) []byte {
	t.Helper()

	spanBytes := make([]byte, pubsub.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))

	cacData := make([]byte, 0, pubsub.SpanSize+len(payload))
	cacData = append(cacData, spanBytes...)
	cacData = append(cacData, payload...)
	ch, err := cac.NewWithDataSpan(cacData)
	if err != nil {
		t.Fatal(err)
	}

	h := swarm.NewHasher()
	_, _ = h.Write(tc.gsocID)
	_, _ = h.Write(ch.Address().Bytes())
	toSign := h.Sum(nil)

	sig, err := tc.signer.Sign(toSign)
	if err != nil {
		t.Fatal(err)
	}

	msg := make([]byte, 0, len(sig)+pubsub.SpanSize+len(payload))
	msg = append(msg, sig...)
	msg = append(msg, spanBytes[:pubsub.SpanSize]...)
	msg = append(msg, payload...)
	return msg
}

func TestFormatBroadcast_Handshake(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)
	mode.SetGsocParams(tc.owner, tc.gsocID)

	rawMsg := []byte("sig_span_payload_placeholder_data_65_8_N")
	sub := pubsub.NewBrokerSubscriber()

	out := mode.FormatBroadcast(sub, rawMsg)

	if out[0] != pubsub.MsgTypeHandshake {
		t.Fatalf("expected type 0x%02x, got 0x%02x", pubsub.MsgTypeHandshake, out[0])
	}

	gotID := out[1 : 1+pubsub.IDSize]
	if !bytes.Equal(gotID, tc.gsocID) {
		t.Fatalf("gsocID mismatch: got %x want %x", gotID, tc.gsocID)
	}

	gotOwner := out[1+pubsub.IDSize : 1+pubsub.IDSize+pubsub.OwnerSize]
	if !bytes.Equal(gotOwner, tc.owner) {
		t.Fatalf("owner mismatch: got %x want %x", gotOwner, tc.owner)
	}

	gotPayload := out[1+pubsub.IDSize+pubsub.OwnerSize:]
	if !bytes.Equal(gotPayload, rawMsg) {
		t.Fatalf("payload mismatch: got %x want %x", gotPayload, rawMsg)
	}
}

func TestFormatBroadcast_HandshakeOnce(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)
	mode.SetGsocParams(tc.owner, tc.gsocID)

	rawMsg := []byte("data")
	sub := pubsub.NewBrokerSubscriber()

	// First call: handshake
	first := mode.FormatBroadcast(sub, rawMsg)
	if first[0] != pubsub.MsgTypeHandshake {
		t.Fatalf("expected handshake type on first call")
	}

	// Second call: data only
	second := mode.FormatBroadcast(sub, rawMsg)
	if second[0] != pubsub.MsgTypeData {
		t.Fatalf("expected data type on second call, got 0x%02x", second[0])
	}
	if !bytes.Equal(second[1:], rawMsg) {
		t.Fatalf("data payload mismatch")
	}
}

func TestFormatBroadcast_DataAfterHandshake(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)
	mode.SetGsocParams(tc.owner, tc.gsocID)

	rawMsg := []byte("payload")
	sub := pubsub.NewBrokerSubscriber()
	sub.SetHandshakeDone()

	out := mode.FormatBroadcast(sub, rawMsg)

	if out[0] != pubsub.MsgTypeData {
		t.Fatalf("expected type 0x%02x, got 0x%02x", pubsub.MsgTypeData, out[0])
	}
	if !bytes.Equal(out[1:], rawMsg) {
		t.Fatalf("data payload mismatch")
	}
}

func TestReadBrokerMessage_Ping(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)

	stream := newReaderStream([]byte{pubsub.MsgTypePing}, nil)
	msg, err := mode.ReadBrokerMessage(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Fatalf("expected nil msg for ping, got %v", msg)
	}
}

func TestReadBrokerMessage_Handshake(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)

	payload := []byte("sziasztok!")
	publisherFrame := buildPublisherMsg(t, tc, payload)

	// Assemble handshake message: [0x00][gsocID(32B)][owner(20B)][sig(65B)][span(8B)][payload]
	streamData := make([]byte, 0, 1+len(tc.gsocID)+len(tc.owner)+len(publisherFrame))
	streamData = append(streamData, pubsub.MsgTypeHandshake)
	streamData = append(streamData, tc.gsocID...)
	streamData = append(streamData, tc.owner...)
	streamData = append(streamData, publisherFrame...)

	stream := newReaderStream(streamData, nil)
	msg, err := mode.ReadBrokerMessage(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message from handshake")
	}

	// Returned msg is [sig(65B)][span(8B)][payload]; verify it matches publisherFrame
	if !bytes.Equal(msg, publisherFrame) {
		t.Fatalf("returned message mismatch")
	}
}

func TestReadBrokerMessage_Data(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)
	mode.SetGsocParams(tc.owner, tc.gsocID)

	payload := []byte("data message")
	publisherFrame := buildPublisherMsg(t, tc, payload)

	// Assemble data message: [0x01][sig(65B)][span(8B)][payload]
	streamData := make([]byte, 0, 1+len(publisherFrame))
	streamData = append(streamData, pubsub.MsgTypeData)
	streamData = append(streamData, publisherFrame...)

	stream := newReaderStream(streamData, nil)
	msg, err := mode.ReadBrokerMessage(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(msg, publisherFrame) {
		t.Fatalf("data message mismatch: got %x want %x", msg, publisherFrame)
	}
}

func TestReadBrokerMessage_InvalidSig(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)
	mode.SetGsocParams(tc.owner, tc.gsocID)

	payload := []byte("bad sig")
	spanBytes := make([]byte, pubsub.SpanSize)
	binary.LittleEndian.PutUint64(spanBytes, uint64(len(payload)))
	badSig := make([]byte, pubsub.SigSize) // all zeros — invalid signature

	streamData := make([]byte, 0, 1+len(badSig)+len(spanBytes)+len(payload))
	streamData = append(streamData, pubsub.MsgTypeData)
	streamData = append(streamData, badSig...)
	streamData = append(streamData, spanBytes...)
	streamData = append(streamData, payload...)

	stream := newReaderStream(streamData, nil)
	_, err := mode.ReadBrokerMessage(stream)
	if err == nil {
		t.Fatal("expected error for invalid signature, got nil")
	}
}

func TestSetGsocParams_AddressMismatch(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)

	// Use a different ID that does not hash to topicAddr with tc.owner
	wrongID := make([]byte, swarm.HashSize)
	wrongID[0] = 0xff

	mode.SetGsocParams(tc.owner, wrongID)

	if mode.GsocOwner() != nil {
		t.Fatal("expected gsocOwner to remain nil after address mismatch")
	}
}

func TestSetGsocParams_ValidParams(t *testing.T) {
	t.Parallel()

	tc := newSocTestCtx(t)
	mode := pubsub.NewGSOCEphemeralMode(tc.topicAddr[:], log.Noop)

	mode.SetGsocParams(tc.owner, tc.gsocID)

	if mode.GsocOwner() == nil {
		t.Fatal("expected gsocOwner to be set after valid params")
	}
	if !bytes.Equal(mode.GsocOwner(), tc.owner) {
		t.Fatalf("gsocOwner mismatch: got %x want %x", mode.GsocOwner(), tc.owner)
	}
}

func TestSubscriberConn_FanOut(t *testing.T) {
	t.Parallel()

	sc := pubsub.NewTestSubscriberConn(log.Noop)

	id1, ch1 := sc.Subscribe()
	id2, ch2 := sc.Subscribe()
	defer sc.Unsubscribe(id1)
	defer sc.Unsubscribe(id2)

	msg := []byte("broadcast")
	sc.FanOut(msg)

	for i, ch := range []<-chan []byte{ch1, ch2} {
		select {
		case got := <-ch:
			if !bytes.Equal(got, msg) {
				t.Fatalf("subscriber %d: got %x want %x", i, got, msg)
			}
		default:
			t.Fatalf("subscriber %d: no message received", i)
		}
	}
}

func TestSubscriberConn_Unsubscribe(t *testing.T) {
	t.Parallel()

	sc := pubsub.NewTestSubscriberConn(log.Noop)

	id, ch := sc.Subscribe()
	sc.Unsubscribe(id)

	// Channel should be closed after unsubscribe.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	default:
		t.Fatal("expected channel to be closed (not blocking)")
	}
}

func TestSubscriberConn_FanOut_SkipsFullChannel(t *testing.T) {
	t.Parallel()

	sc := pubsub.NewTestSubscriberConn(log.Noop)

	id, ch := sc.Subscribe()
	defer sc.Unsubscribe(id)

	// Fill the channel to capacity (16 slots).
	for i := range cap(ch) {
		sc.FanOut([]byte{byte(i)})
	}

	// This should not block even though the channel is full.
	sc.FanOut([]byte("overflow"))
}

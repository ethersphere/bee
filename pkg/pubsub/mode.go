// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var ErrInvalidSignature = errors.New("pubsub: invalid SOC signature")

const (
	// P2P headers
	HeaderGsocOwner = "pubsub-gsoc-owner"
	HeaderGsocID    = "pubsub-gsoc-id"
)

// ModeID identifies a pubsub mode.
type ModeID uint8

// Mode defines mode-specific behavior for the pubsub protocol.
// Each mode determines its own roles, wire format, and message handling.
type Mode interface {
	ID() ModeID
	TopicAddress() swarm.Address
	Connect(ctx context.Context, p p2p.Streamer, overlay swarm.Address, opts ConnectOptions) (p2p.Stream, error)

	// Broker side - publisher
	ValidatePublisher(bc *brokerConn, headers p2p.Headers) error
	ReadPublisherMessage(stream p2p.Stream) ([]byte, error)

	// Broker side - broadcast
	FormatBroadcast(bt *brokerConn, sub *brokerSubscriber, rawMsg []byte) []byte

	// Subscriber/WS side
	ReadBrokerMessage(stream p2p.Stream) ([]byte, error)
}

// --- GSOC Ephemeral Mode (mode 1) ---

const (
	// Message types (Broker → Subscriber)
	MsgTypeHandshake byte = 0x01
	MsgTypeData      byte = 0x02
)

// GSOCEphemeralMode implements Mode for GSOC ephemeral messaging.
type GSOCEphemeralMode struct {
	topicAddress      swarm.Address
	gsocOwner         []byte
	gsocID            []byte
	handshakeHappened bool // true after the first message from broker. sends validation metadata
}

var _ Mode = (*GSOCEphemeralMode)(nil)

func NewGSOCEphemeralMode(topicAddress []byte) *GSOCEphemeralMode {
	return &GSOCEphemeralMode{
		topicAddress: swarm.NewAddress(topicAddress),
	}
}

func (m *GSOCEphemeralMode) ID() ModeID { return ModeGSOCEphemeral }

func (m *GSOCEphemeralMode) TopicAddress() swarm.Address { return m.topicAddress.Clone() }

func (m *GSOCEphemeralMode) Connect(ctx context.Context, p p2p.Streamer, overlay swarm.Address, opts ConnectOptions) (p2p.Stream, error) {
	var rw byte
	if opts.ReadWrite {
		rw = 1
	}
	headers := p2p.Headers{
		HeaderTopicAddress: m.topicAddress.Bytes(),
		HeaderMode:         {byte(m.ID())},
		HeaderReadWrite:    {rw},
	}
	if len(opts.GsocOwner) > 0 {
		headers[HeaderGsocOwner] = opts.GsocOwner
	}
	if len(opts.GsocID) > 0 {
		headers[HeaderGsocID] = opts.GsocID
	}
	return p.NewStream(ctx, overlay, headers, protocolName, protocolVersion, streamName)
}

// ValidatePublisher sets SOC parameters on the broker side so it can validate the messages.
func (m *GSOCEphemeralMode) ValidatePublisher(bc *brokerConn, headers p2p.Headers) error {
	gsocOwner := headers[HeaderGsocOwner]
	gsocID := headers[HeaderGsocID]

	bc.mu.Lock()
	m.setGsocParams(gsocOwner, gsocID)
	set := m.gsocID != nil
	bc.mu.Unlock()

	if !set {
		return ErrWrongHeaders
	}
	return nil
}

// FormatBroadcast formats a raw publisher message for delivery to a subscriber.
// First delivery to each subscriber includes a handshake with SOC identity; subsequent are data-only.
func (m *GSOCEphemeralMode) FormatBroadcast(bt *brokerConn, sub *brokerSubscriber, rawMsg []byte) []byte {
	if !m.handshakeHappened {
		// Handshake: [1B type=0x01][32B SOC ID][20B owner][65B sig][4B span][NB payload]
		msg := make([]byte, 1+IDSize+OwnerSize+len(rawMsg))
		msg[0] = MsgTypeHandshake
		copy(msg[1:1+IDSize], m.gsocID)
		copy(msg[1+IDSize:1+IDSize+OwnerSize], m.gsocOwner)
		copy(msg[1+IDSize+OwnerSize:], rawMsg)
		m.handshakeHappened = true
		return msg
	}

	// Data: [1B type=0x02][65B sig][4B span][NB payload]
	msg := make([]byte, 1+len(rawMsg))
	msg[0] = MsgTypeData
	copy(msg[1:], rawMsg)
	return msg
}

// ReadPublisherMessage reads [65B sig][4B span][NB payload (max 4KB)] from the stream,
// constructs and validates the SOC chunk and returns that.
func (m *GSOCEphemeralMode) ReadPublisherMessage(stream p2p.Stream) ([]byte, error) {
	sig := make([]byte, SigSize)
	if _, err := io.ReadFull(stream, sig); err != nil {
		return nil, err
	}
	spanBytes := make([]byte, SpanSize)
	if _, err := io.ReadFull(stream, spanBytes); err != nil {
		return nil, err
	}
	span := min(binary.LittleEndian.Uint32(spanBytes), MaxPayload)

	payload := make([]byte, span)
	if _, err := io.ReadFull(stream, payload); err != nil {
		return nil, err
	}

	// Construct SOC chunk with the known topic address: [ID (32B)][sig (65B)][span (4B)][payload]
	// and validate whether message is valid
	socData := make([]byte, IDSize+SigSize+SpanSize+int(span))
	copy(socData, m.gsocID)
	copy(socData[IDSize:], sig)
	copy(socData[IDSize+SigSize:], spanBytes)
	copy(socData[IDSize+SigSize+SpanSize:], payload)

	if !soc.Valid(swarm.NewChunk(m.topicAddress, socData)) {
		return nil, ErrInvalidSignature
	}

	return socData[IDSize:], nil
}

// ReadBrokerMessage reads one broker→subscriber message and verifies it
func (m *GSOCEphemeralMode) ReadBrokerMessage(stream p2p.Stream) ([]byte, error) {
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(stream, typeBuf); err != nil {
		return nil, err
	}

	switch typeBuf[0] {
	case MsgTypeHandshake:
		socID := make([]byte, IDSize)
		if _, err := io.ReadFull(stream, socID); err != nil {
			return nil, fmt.Errorf("read SOC ID: %w", err)
		}
		ownerAddr := make([]byte, OwnerSize)
		if _, err := io.ReadFull(stream, ownerAddr); err != nil {
			return nil, fmt.Errorf("read owner addr: %w", err)
		}
		m.setGsocParams(ownerAddr, socID)

		return m.ReadPublisherMessage(stream) // same as publisher message at this point

	case MsgTypeData:
		if m.gsocID == nil {
			return nil, fmt.Errorf("pubsub: data message before handshake")
		}
		return m.ReadPublisherMessage(stream)

	default:
		return nil, fmt.Errorf("pubsub: unknown message type: 0x%02x", typeBuf[0])
	}
}

// setGsocParams sets the GSOC recurring parameters so that messages don't need to include them.
func (m *GSOCEphemeralMode) setGsocParams(gsocOwner, gsocID []byte) {
	if m.gsocOwner != nil {
		return
	}
	// Verify got socId and address match with topicaddress
	addr, err := soc.CreateAddress(gsocID, gsocOwner)
	if err != nil || !bytes.Equal(addr.Bytes(), m.topicAddress.Bytes()) {
		return
	}

	m.gsocOwner = make([]byte, OwnerSize)
	copy(m.gsocOwner, gsocOwner)
	m.gsocID = make([]byte, IDSize)
	copy(m.gsocID, gsocID)
}

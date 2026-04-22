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
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
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

	// Subscriber side - outbound connection to broker
	Connect(ctx context.Context, p p2p.Streamer, overlay swarm.Address, opts ConnectOptions) (p2p.Stream, error)
	CreateSubscriberConn(stream p2p.Stream, overlay swarm.Address, cancel context.CancelFunc)
	GetSubscriberConn() *SubscriberConn
	RemoveSubscriberConn()
	ReadBrokerMessage(stream p2p.Stream) ([]byte, error)

	// Broker side - handles incoming streams (publisher and subscriber)
	HandleBroker(ctx context.Context, peer p2p.Peer, stream p2p.Stream, headers p2p.Headers) error
	SubscriberCount() int
	SubscriberOverlays() []string
}

// --- GSOC Ephemeral Mode (mode 1) ---

const (
	// Message types (Broker → Subscriber)
	MsgTypeHandshake byte = 0x01
	MsgTypeData      byte = 0x02
	MsgTypePing      byte = 0x03

	// Ping interval for keeping p2p streams alive.
	streamPingInterval = 30 * time.Second
)

// GSOCEphemeralMode implements Mode for GSOC ephemeral messaging.
type GSOCEphemeralMode struct {
	mu             sync.RWMutex
	topicAddress   swarm.Address
	gsocOwner      []byte
	gsocID         []byte
	logger         log.Logger
	subscribers    map[string]*brokerSubscriber
	subscriberConn *SubscriberConn
}

// brokerSubscriber holds a subscriber's stream and outgoing message channel.
type brokerSubscriber struct {
	overlay           swarm.Address
	stream            p2p.Stream
	outCh             chan []byte
	cancel            context.CancelFunc
	handshakeHappened bool
}

// SubscriberConn represents the subscriber-side connection to a broker.
type SubscriberConn struct {
	Stream  p2p.Stream
	Overlay swarm.Address
	Cancel  context.CancelFunc
}

var _ Mode = (*GSOCEphemeralMode)(nil)

func NewGSOCEphemeralMode(topicAddress []byte, logger log.Logger) *GSOCEphemeralMode {
	return &GSOCEphemeralMode{
		topicAddress: swarm.NewAddress(topicAddress),
		logger:       logger,
		subscribers:  make(map[string]*brokerSubscriber),
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

// validatePublisher sets SOC parameters on the broker side so it can validate the messages.
func (m *GSOCEphemeralMode) validatePublisher(headers p2p.Headers) error {
	gsocOwner := headers[HeaderGsocOwner]
	gsocID := headers[HeaderGsocID]

	m.mu.Lock()
	m.setGsocParams(gsocOwner, gsocID)
	set := m.gsocID != nil
	m.mu.Unlock()

	if !set {
		return ErrWrongHeaders
	}
	return nil
}

// FormatBroadcast formats a raw publisher message for delivery to a subscriber.
// First delivery to each subscriber includes a handshake with SOC identity; subsequent are data-only.
func (m *GSOCEphemeralMode) formatBroadcast(sub *brokerSubscriber, rawMsg []byte) []byte {
	if !sub.handshakeHappened {
		// Handshake: [1B type=0x01][32B SOC ID][20B owner][65B sig][4B span][NB payload]
		msg := make([]byte, 1+IDSize+OwnerSize+len(rawMsg))
		msg[0] = MsgTypeHandshake
		copy(msg[1:1+IDSize], m.gsocID)
		copy(msg[1+IDSize:1+IDSize+OwnerSize], m.gsocOwner)
		copy(msg[1+IDSize+OwnerSize:], rawMsg)
		sub.handshakeHappened = true
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
	case MsgTypePing:
		m.logger.Debug("received ping from broker")
		return nil, nil

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

// SubscriberCount returns the number of active subscribers.
func (m *GSOCEphemeralMode) SubscriberCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscribers)
}

// SubscriberOverlays returns the overlay addresses of all active subscribers.
func (m *GSOCEphemeralMode) SubscriberOverlays() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	overlays := make([]string, 0, len(m.subscribers))
	for _, sub := range m.subscribers {
		overlays = append(overlays, sub.overlay.String())
	}
	return overlays
}

// broadcast sends a message to all subscribers.
func (m *GSOCEphemeralMode) broadcast(rawMsg []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("broadcasting to subscribers", "count", len(m.subscribers), "size", len(rawMsg))
	for _, sub := range m.subscribers {
		msg := m.formatBroadcast(sub, rawMsg)

		select {
		case sub.outCh <- msg:
			m.logger.Info("message enqueued for subscriber", "peer", sub.overlay, "size", len(msg))
		default:
			m.logger.Warning("subscriber message queue full, dropping message", "peer", sub.overlay)
		}
	}
}

// HandleBroker handles an incoming broker-side stream, dispatching to publisher or subscriber handling.
func (m *GSOCEphemeralMode) HandleBroker(ctx context.Context, peer p2p.Peer, stream p2p.Stream, headers p2p.Headers) error {
	rwBytes := headers[HeaderReadWrite]
	m.logger.Debug("reading rw header", "peer", peer.Address)
	if len(rwBytes) != 1 {
		_ = stream.Reset()
		return ErrWrongHeaders
	}
	if rwBytes[0] == 1 {
		return m.handlePublisher(ctx, peer, stream, headers)
	}
	m.logger.Debug("handling as subscriber", "peer", peer.Address)
	return m.handleSubscriber(ctx, peer, stream)
}

func (m *GSOCEphemeralMode) handleSubscriber(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
	subCtx, cancel, unregister := m.registerSubscriber(ctx, peer.Address, stream)
	defer cancel()
	defer unregister()

	m.logger.Debug("subscriber connected", "peer", peer.Address, "topic", m.TopicAddress())

	<-subCtx.Done()
	if errors.Is(subCtx.Err(), context.Canceled) {
		return nil
	}
	return subCtx.Err()
}

func (m *GSOCEphemeralMode) handlePublisher(ctx context.Context, peer p2p.Peer, stream p2p.Stream, headers p2p.Headers) error {
	if err := m.validatePublisher(headers); err != nil {
		_ = stream.Reset()
		return err
	}

	partCtx, cancel, unregister := m.registerSubscriber(ctx, peer.Address, stream)
	defer cancel()
	defer unregister()

	m.logger.Debug("publisher connected", "peer", peer.Address, "topic", m.TopicAddress())

	for {
		select {
		case <-partCtx.Done():
			if errors.Is(partCtx.Err(), context.Canceled) {
				return nil
			}
			return partCtx.Err()
		default:
		}

		rawMsg, err := m.ReadPublisherMessage(stream)
		if err != nil {
			if errors.Is(err, io.EOF) {
				m.logger.Info("publisher stream EOF", "peer", peer.Address)
				return nil
			}
			return fmt.Errorf("read publisher message: %w", err)
		}

		m.logger.Info("publisher message received", "peer", peer.Address, "size", len(rawMsg))
		m.broadcast(rawMsg)
	}
}

// registerSubscriber adds a peer as a subscriber and starts a write goroutine for it.
func (m *GSOCEphemeralMode) registerSubscriber(ctx context.Context, overlay swarm.Address, stream p2p.Stream) (context.Context, context.CancelFunc, func()) {
	connCtx, cancel := context.WithCancel(ctx)

	sub := &brokerSubscriber{
		overlay: overlay,
		stream:  stream,
		outCh:   make(chan []byte, 256),
		cancel:  cancel,
	}

	overlayKey := overlay.String()
	m.mu.Lock()
	m.subscribers[overlayKey] = sub
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(streamPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case msg := <-sub.outCh:
				if err := writeRaw(stream, msg); err != nil {
					m.logger.Info("broker write to subscriber failed", "peer", sub.overlay, "error", err)
					cancel()
					return
				}
				m.logger.Info("broker wrote to subscriber", "peer", sub.overlay, "size", len(msg))
			case <-ticker.C:
				if err := writeRaw(stream, []byte{MsgTypePing}); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	unregister := func() {
		m.mu.Lock()
		delete(m.subscribers, overlayKey)
		m.mu.Unlock()
	}

	return connCtx, cancel, unregister
}

// CreateSubscriberConn creates and stores the subscriber-side connection in this mode.
func (m *GSOCEphemeralMode) CreateSubscriberConn(stream p2p.Stream, overlay swarm.Address, cancel context.CancelFunc) {
	m.mu.Lock()
	m.subscriberConn = &SubscriberConn{
		Stream:  stream,
		Overlay: overlay,
		Cancel:  cancel,
	}
	m.mu.Unlock()
}

// GetSubscriberConn returns the subscriber-side connection, or nil.
func (m *GSOCEphemeralMode) GetSubscriberConn() *SubscriberConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subscriberConn
}

// RemoveSubscriberConn clears the subscriber-side connection.
func (m *GSOCEphemeralMode) RemoveSubscriberConn() {
	m.mu.Lock()
	m.subscriberConn = nil
	m.mu.Unlock()
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

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	loggerName      = "pubsub"
	protocolName    = "pubsub"
	protocolVersion = "1.0.0"
	streamName      = "msg"

	// p2p stream header keys
	HeaderTopicAddress = "pubsub-topic-address"
	HeaderMode         = "pubsub-mode"
	HeaderReadWrite    = "pubsub-readwrite" // 1 = read+write (publisher), 0 = read-only (subscriber)

	// Mode constants
	ModeGSOCEphemeral ModeID = 1

	// Wire format sizes
	SpanSize   = 4 // pubsub span: uint32 little-endian
	MaxPayload = swarm.ChunkSize
	SigSize    = swarm.SocSignatureSize
	IDSize     = swarm.HashSize
	OwnerSize  = crypto.AddressSize
)

var (
	ErrBrokerDisabled   = errors.New("pubsub: broker mode is disabled")
	ErrMaxConnections   = errors.New("pubsub: max connections reached")
	ErrInvalidHandshake = errors.New("pubsub: handshake verification failed")
	ErrWrongHeaders     = errors.New("pubsub: wrong required headers")
	ErrTopicMismatch    = errors.New("pubsub: topic address mismatch")
)

func newMode(topicAddr [32]byte, modeID ModeID) (Mode, error) {
	switch modeID {
	case ModeGSOCEphemeral:
		return NewGSOCEphemeralMode(topicAddr[:]), nil
	default:
		return nil, fmt.Errorf("pubsub: unknown mode: %d", modeID)
	}
}

// ConnectOptions carries optional mode-specific parameters for Connect.
type ConnectOptions struct {
	ReadWrite bool // true = publisher (read+write), false = subscriber (read-only)
	GsocOwner []byte
	GsocID    []byte
}

// TopicInfo describes a topic for the list endpoint.
type TopicInfo struct {
	TopicAddress string   `json:"topicAddress"`
	Mode         ModeID   `json:"mode"`
	Role         string   `json:"role"`
	Connections  []string `json:"connections"`
}

// brokerSubscriber holds a subscriber's stream and outgoing message channel.
type brokerSubscriber struct {
	overlay swarm.Address
	stream  p2p.Stream
	outCh   chan []byte
	cancel  context.CancelFunc
}

// brokerConn manages all connections for a single topic on the broker side.
type brokerConn struct {
	mu          sync.RWMutex
	mode        Mode
	subscribers map[string]*brokerSubscriber
}

// SubscriberConn represents the subscriber-side connection to a broker.
type SubscriberConn struct {
	Stream    p2p.Stream
	TopicAddr [32]byte
	Mode      Mode
	Overlay   swarm.Address
	Cancel    context.CancelFunc
}

// P2P groups the p2p capabilities needed by the pubsub service.
type P2P interface {
	p2p.Service
	p2p.Streamer
}

// Service is the pubsub protocol service.
type Service struct {
	mu              sync.RWMutex
	p2p             P2P
	logger          log.Logger
	brokerMode      bool
	maxConns        int
	brokerConns     map[[32]byte]*brokerConn     // topic address -> broker connection
	subscriberConns map[[32]byte]*SubscriberConn // topic address -> subscriber connection
}

func New(p2p P2P, logger log.Logger, brokerMode bool, maxConns int) *Service {
	s := &Service{
		p2p:             p2p,
		logger:          logger.WithName(loggerName).Register(),
		brokerMode:      brokerMode,
		maxConns:        maxConns,
		brokerConns:     make(map[[32]byte]*brokerConn),
		subscriberConns: make(map[[32]byte]*SubscriberConn),
	}
	return s
}

// Protocol returns the p2p protocol spec.
func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Handler: s.brokerHandler,
			},
		},
	}
}

// Connect establishes a subscriber connection to a broker peer.
func (s *Service) Connect(ctx context.Context, underlay ma.Multiaddr, topicAddr [32]byte, modeID ModeID, opts ConnectOptions) (*SubscriberConn, error) {
	m, err := newMode(topicAddr, modeID)
	if err != nil {
		return nil, err
	}

	s.logger.Info("connecting to broker peer", "underlay", underlay)
	bzzAddr, err := s.p2p.Connect(ctx, []ma.Multiaddr{underlay})
	if err != nil && !errors.Is(err, p2p.ErrAlreadyConnected) {
		return nil, fmt.Errorf("connect to peer: %w", err)
	}
	s.logger.Info("connected to broker peer", "overlay", bzzAddr.Overlay)

	stream, err := m.Connect(ctx, s.p2p, bzzAddr.Overlay, opts)
	if err != nil {
		s.logger.Error(err,"bagoy open stream")
		return nil, fmt.Errorf("open stream: %w", err)
	}

	connCtx, cancel := context.WithCancel(ctx)
	sc := &SubscriberConn{
		Stream:    stream,
		TopicAddr: topicAddr,
		Mode:      m,
		Overlay:   bzzAddr.Overlay,
		Cancel:    cancel,
	}

	s.mu.Lock()
	s.subscriberConns[topicAddr] = sc
	s.mu.Unlock()

	go func() {
		<-connCtx.Done()
		s.mu.Lock()
		delete(s.subscriberConns, topicAddr)
		s.mu.Unlock()
		_ = stream.FullClose()
	}()

	return sc, nil
}

// Topics returns info about all active topics.
func (s *Service) Topics() []TopicInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var topics []TopicInfo

	for addr, bt := range s.brokerConns {
		bt.mu.RLock()
		conns := make([]string, 0, len(bt.subscribers))
		for _, sub := range bt.subscribers {
			conns = append(conns, sub.overlay.String())
		}
		bt.mu.RUnlock()
		topics = append(topics, TopicInfo{
			TopicAddress: fmt.Sprintf("%x", addr),
			Mode:         bt.mode.ID(),
			Role:         "broker",
			Connections:  conns,
		})
	}

	for addr, sc := range s.subscriberConns {
		topics = append(topics, TopicInfo{
			TopicAddress: fmt.Sprintf("%x", addr),
			Mode:         sc.Mode.ID(),
			Role:         "subscriber",
			Connections:  []string{sc.Overlay.String()},
		})
	}

	return topics
}

// brokerHandler handles incoming streams on the broker side.
func (s *Service) brokerHandler(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
	fmt.Println("Broker handler ")
	if !s.brokerMode {
		_ = stream.Reset()
		return ErrBrokerDisabled
	}

	headers := stream.Headers()

	topicAddrBytes := headers[HeaderTopicAddress]
	if len(topicAddrBytes) != IDSize {
		_ = stream.Reset()
		return ErrWrongHeaders
	}
	var topicAddr [32]byte
	copy(topicAddr[:], topicAddrBytes)

	modeBytes := headers[HeaderMode]
	if len(modeBytes) != 1 {
		_ = stream.Reset()
		return ErrWrongHeaders
	}
	bc, err := s.getOrCreateBrokerConn(topicAddr, ModeID(modeBytes[0]))
	if err != nil {
		_ = stream.Reset()
		return err
	}

	rwBytes := headers[HeaderReadWrite]
	fmt.Println("rwBytes header1")
	if len(rwBytes) != 1 {
		_ = stream.Reset()
		return ErrWrongHeaders
	}
	if rwBytes[0] == 1 {
		return s.handlePublisher(ctx, peer, stream, bc, headers)
	}
	fmt.Println("rwBytes header2")
	return s.handleSubscriber(ctx, peer, stream, bc)
}

// registerBrokerConn registers the peer as a broadcast recipient on bc, starts a write goroutine,
// and returns the connection context, its cancel func, and an unregister func to call on exit.
func (s *Service) registerBrokerConn(ctx context.Context, peer p2p.Peer, stream p2p.Stream, bc *brokerConn) (connCtx context.Context, cancel context.CancelFunc, unregister func()) {
	connCtx, cancel = context.WithCancel(ctx)

	conn := &brokerSubscriber{
		overlay: peer.Address,
		stream:  stream,
		outCh:   make(chan []byte, 256),
		cancel:  cancel,
	}

	overlayKey := peer.Address.String()
	bc.mu.Lock()
	bc.subscribers[overlayKey] = conn
	bc.mu.Unlock()

	go func() {
		for {
			select {
			case <-connCtx.Done():
				return
			case msg := <-conn.outCh:
				if err := writeRaw(stream, msg); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	unregister = func() {
		bc.mu.Lock()
		delete(bc.subscribers, overlayKey)
		bc.mu.Unlock()
	}
	return
}

func (s *Service) handleSubscriber(ctx context.Context, peer p2p.Peer, stream p2p.Stream, bc *brokerConn) error {
	bc.mu.RLock()
	connCount := len(bc.subscribers)
	bc.mu.RUnlock()
	if s.maxConns > 0 && connCount >= s.maxConns {
		_ = stream.Reset()
		return ErrMaxConnections
	}

	subCtx, cancel, unregister := s.registerBrokerConn(ctx, peer, stream, bc)
	defer cancel()
	defer unregister()

	s.logger.Debug("subscriber connected", "peer", peer.Address, "topic", bc.mode.TopicAddress())

	<-subCtx.Done()
	if errors.Is(subCtx.Err(), context.Canceled) {
		return nil
	}
	return subCtx.Err()
}

func (s *Service) handlePublisher(ctx context.Context, peer p2p.Peer, stream p2p.Stream, bc *brokerConn, headers p2p.Headers) error {
	if err := bc.mode.ValidatePublisher(bc, headers); err != nil {
		_ = stream.Reset()
		return err
	}

	partCtx, cancel, unregister := s.registerBrokerConn(ctx, peer, stream, bc)
	defer cancel()
	defer unregister()

	s.logger.Debug("publisher connected", "peer", peer.Address, "topic", bc.mode.TopicAddress())

	for {
		select {
		case <-partCtx.Done():
			if errors.Is(partCtx.Err(), context.Canceled) {
				return nil
			}
			return partCtx.Err()
		default:
		}

		rawMsg, err := bc.mode.ReadPublisherMessage(stream)
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.logger.Info("publisher stream EOF", "peer", peer.Address)
				return nil
			}
			return fmt.Errorf("read publisher message: %w", err)
		}

		s.logger.Info("publisher message received", "peer", peer.Address, "size", len(rawMsg))
		s.broadcastToSubscribers(bc, rawMsg)
	}
}

// broadcastToSubscribers sends a message to all subscribers of a topic.
func (s *Service) broadcastToSubscribers(bc *brokerConn, rawMsg []byte) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	s.logger.Info("broadcasting to subscribers", "count", len(bc.subscribers), "size", len(rawMsg))
	for _, sub := range bc.subscribers {
		msg := bc.mode.FormatBroadcast(bc, sub, rawMsg)

		select {
		case sub.outCh <- msg:
			s.logger.Info("message enqueued for subscriber", "peer", sub.overlay, "size", len(msg))
		default:
			s.logger.Warning("subscriber message queue full, dropping message", "peer", sub.overlay)
		}
	}
}

func (s *Service) getOrCreateBrokerConn(topicAddr [32]byte, modeID ModeID) (*brokerConn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if bt, ok := s.brokerConns[topicAddr]; ok {
		return bt, nil
	}

	m, err := newMode(topicAddr, modeID)
	if err != nil {
		return nil, err
	}

	bc := &brokerConn{
		mode:        m,
		subscribers: make(map[string]*brokerSubscriber),
	}
	s.brokerConns[topicAddr] = bc
	return bc, nil
}

// writeRaw writes raw bytes to the stream.
func writeRaw(stream p2p.Stream, data []byte) error {
	c := 0
	for c < len(data) {
		n, err := stream.Write(data[c:])
		if err != nil {
			return err
		}
		c += n
	}
	return nil
}

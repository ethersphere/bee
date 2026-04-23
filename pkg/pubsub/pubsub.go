// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"context"
	"errors"
	"fmt"
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
	SpanSize   = swarm.SpanSize // pubsub span: 8-byte little-endian uint64 (matches bee-js Span.LENGTH)
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

func newMode(topicAddr [32]byte, modeID ModeID, logger log.Logger) (Mode, error) {
	switch modeID {
	case ModeGSOCEphemeral:
		return NewGSOCEphemeralMode(topicAddr[:], logger), nil
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

// TopicModeKey is a composite key for identifying a mode instance per topic.
type TopicModeKey struct {
	TopicAddr [32]byte
	ModeID    ModeID
}

// P2P groups the p2p capabilities needed by the pubsub service.
type P2P interface {
	p2p.Service
	p2p.Streamer
}

// Service is the pubsub protocol service.
type Service struct {
	mu         sync.RWMutex
	p2p        P2P
	logger     log.Logger
	brokerMode bool
	maxConns   int
	modes      map[TopicModeKey]Mode // (topic, mode) -> mode instance
}

func New(p2p P2P, logger log.Logger, brokerMode bool, maxConns int) *Service {
	s := &Service{
		p2p:        p2p,
		logger:     logger.WithName(loggerName).Register(),
		brokerMode: brokerMode,
		maxConns:   maxConns,
		modes:      make(map[TopicModeKey]Mode),
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
func (s *Service) Connect(ctx context.Context, underlay ma.Multiaddr, topicAddr [32]byte, modeID ModeID, opts ConnectOptions) (Mode, error) {
	key := TopicModeKey{TopicAddr: topicAddr, ModeID: modeID}
	m, err := s.getOrCreateMode(key)
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
		s.logger.Error(err, "open stream failed")
		return nil, fmt.Errorf("open stream: %w", err)
	}

	connCtx, cancel := context.WithCancel(ctx)
	sc := m.CreateSubscriberConn(stream, bzzAddr.Overlay, cancel)

	go func() {
		<-connCtx.Done()
		_ = stream.FullClose()
		m.RemoveSubscriberConn(sc)
	}()

	return m, nil
}

// Topics returns info about all active topics.
func (s *Service) Topics() []TopicInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var topics []TopicInfo

	for key, m := range s.modes {
		info := TopicInfo{
			TopicAddress: fmt.Sprintf("%x", key.TopicAddr),
			Mode:         m.ID(),
			Connections:  m.SubscriberOverlays(),
		}
		sc := m.GetSubscriberConn()
		switch {
		case m.SubscriberCount() > 0 && sc != nil:
			info.Role = "broker+subscriber"
			info.Connections = append(info.Connections, sc.Overlay.String())
		case m.SubscriberCount() > 0:
			info.Role = "broker"
		case sc != nil:
			info.Role = "subscriber"
			info.Connections = []string{sc.Overlay.String()}
		default:
			continue
		}
		topics = append(topics, info)
	}

	return topics
}

// brokerHandler handles incoming streams on the broker side.
func (s *Service) brokerHandler(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
	s.logger.Info("broker handler invoked", "peer", peer.Address)
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
	key := TopicModeKey{TopicAddr: topicAddr, ModeID: ModeID(modeBytes[0])}
	m, err := s.getOrCreateMode(key)
	if err != nil {
		_ = stream.Reset()
		return err
	}

	if s.maxConns > 0 && m.SubscriberCount() >= s.maxConns {
		_ = stream.Reset()
		return ErrMaxConnections
	}

	return m.HandleBroker(ctx, peer, stream, headers)
}

func (s *Service) getOrCreateMode(key TopicModeKey) (Mode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m, ok := s.modes[key]; ok {
		return m, nil
	}

	m, err := newMode(key.TopicAddr, key.ModeID, s.logger)
	if err != nil {
		return nil, err
	}

	s.modes[key] = m
	return m, nil
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

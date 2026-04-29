// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bzz"
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

	// Service-level broker message types.
	MsgTypePing byte = 0x03

	// streamPingInterval is how often the broker sends a keepalive ping to each subscriber.
	streamPingInterval = 30 * time.Second

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

// startBrokerWriter starts a goroutine that drains outCh to stream and sends
// keepalive pings on every streamPingInterval tick.
func startBrokerWriter(ctx context.Context, cancel context.CancelFunc, stream p2p.Stream, outCh <-chan []byte, overlay swarm.Address, logger log.Logger) {
	go func() {
		ticker := time.NewTicker(streamPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-outCh:
				if err := writeRaw(stream, msg); err != nil {
					logger.Info("broker write to subscriber failed", "peer", overlay, "error", err)
					cancel()
					return
				}
				logger.Info("broker wrote to subscriber", "peer", overlay, "size", len(msg))
			case <-ticker.C:
				if err := writeRaw(stream, []byte{MsgTypePing}); err != nil {
					cancel()
					return
				}
			}
		}
	}()
}

// readServiceMessage handles broker wire messages that are common to all modes.
// Returns (handled, err). The caller should return (nil, err) when handled is true.
func readServiceMessage(typeBuf byte) (handled bool, err error) {
	switch typeBuf {
	case MsgTypePing:
		return true, nil
	}
	return false, nil
}

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
	// ConnectAllowLight dials a peer, accepting it even if it identifies
	// itself as a light node (broker peers may run in light-node mode).
	ConnectAllowLight(ctx context.Context, addrs []ma.Multiaddr) (*bzz.Address, error)
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
	bzzAddr, err := s.p2p.ConnectAllowLight(ctx, []ma.Multiaddr{underlay})
	if err != nil && !errors.Is(err, p2p.ErrAlreadyConnected) {
		return nil, fmt.Errorf("connect to peer: %w", err)
	}
	s.logger.Info("connected to broker peer", "overlay", bzzAddr.Overlay)

	var sc *SubscriberConn
	if existing := m.GetSubscriberConn(); existing != nil {
		// Reuse the existing p2p stream — no new broker-side stream, just bump the ref count.
		sc = m.CreateSubscriberConn(existing.Stream, bzzAddr.Overlay)
	} else {
		stream, err := m.Connect(ctx, s.p2p, bzzAddr.Overlay, opts)
		if err != nil {
			s.logger.Error(err, "open stream failed")
			return nil, fmt.Errorf("open stream: %w", err)
		}
		sc = m.CreateSubscriberConn(stream, bzzAddr.Overlay)
		if sc.Stream != stream {
			// Race: another goroutine created the conn between our check and create.
			_ = stream.FullClose()
		}
	}

	go func() {
		<-ctx.Done()
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

// brokerSubscriber holds a subscriber's stream and outgoing message channel.
type brokerSubscriber struct {
	overlay           swarm.Address
	stream            p2p.Stream
	outCh             chan []byte
	cancel            context.CancelFunc
	handshakeHappened bool
}

// SubscriberConn represents the shared subscriber-side p2p stream to a broker.
// Multiple WebSocket sessions can attach to one SubscriberConn via the mux.
type SubscriberConn struct {
	Stream  p2p.Stream
	Overlay swarm.Address

	refs    int // number of active WS sessions; protected by the owning mode's mu
	writeMu sync.Mutex
	subsMu  sync.Mutex
	subs    map[uint64]chan []byte
	nextID  uint64
	logger  log.Logger
}

// Subscribe registers a new WS session and returns its per-session message channel.
func (sc *SubscriberConn) Subscribe() (uint64, <-chan []byte) {
	sc.subsMu.Lock()
	defer sc.subsMu.Unlock()
	id := sc.nextID
	sc.nextID++
	ch := make(chan []byte, 16)
	sc.subs[id] = ch
	return id, ch
}

// Unsubscribe removes the WS session channel and closes it.
func (sc *SubscriberConn) Unsubscribe(id uint64) {
	sc.subsMu.Lock()
	defer sc.subsMu.Unlock()
	if ch, ok := sc.subs[id]; ok {
		close(ch)
		delete(sc.subs, id)
	}
}

// fanOut broadcasts a message to all registered WS session channels.
func (sc *SubscriberConn) fanOut(msg []byte) {
	sc.subsMu.Lock()
	defer sc.subsMu.Unlock()
	for _, ch := range sc.subs {
		select {
		case ch <- msg:
		default:
			sc.logger.Warning("pubsub: subscriber ws channel full, dropping message")
		}
	}
}

func (sc *SubscriberConn) closeAll() {
	sc.subsMu.Lock()
	defer sc.subsMu.Unlock()
	for id, ch := range sc.subs {
		close(ch)
		delete(sc.subs, id)
	}
}

// WriteToStream serializes concurrent writes from multiple WS sessions.
func (sc *SubscriberConn) WriteToStream(data []byte) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()
	return writeRaw(sc.Stream, data)
}

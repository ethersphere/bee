// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pingpong exposes the simple ping-pong protocol
// which measures round-trip-time with other peers.
package layer2

import (
	"context"
	"encoding/binary"
	"fmt"
	"reflect"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

const (
	loggerName       = "layer2"
	protocolName     = "layer2"
	p2pMessageHeader = "layer2-message-data"
	protocolVersion  = "1.0.0"
)

var p2pMessageHeaderBytes = []byte(p2pMessageHeader)

// connection is an active p2p connection with a node with all layer 2 handlers and write interface
type connection struct {
	stream   p2p.Stream
	handlers []msgHandler
}

func (conn *connection) SendMessage(ctx context.Context, data []byte) error {
	c := 0
	lenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBytes, uint32(len(data)))
	n, err := conn.stream.Write(lenBytes)
	if err != nil {
		return err
	}

	for c < len(data) {
		n, err = conn.stream.Write(data[c:])
		if err != nil {
			return err
		}
		c += n
	}

	return nil
}

type msgHandler func(swarm.Address, []byte)

type protocolService struct {
	mu sync.RWMutex

	streamName string
	streamer   p2p.Streamer
	logger     log.Logger
	conns      map[[32]byte]*connection // byte overlay to connections
	handlers   []msgHandler
}

func NewProtocolService(streamName string, streamer p2p.Streamer, logger log.Logger) protocolService {
	return protocolService{
		streamName: streamName,
		streamer:   streamer,
		logger:     logger.WithName(loggerName + "_" + streamName).Register(),
		conns:      make(map[[32]byte]*connection),
		handlers:   make([]msgHandler, 0),
	}
}

// GetConnection creates stream to the passed peer address
func (s *protocolService) GetConnection(ctx context.Context, overlay swarm.Address) (conn connection, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.conns[[32]byte(overlay.Bytes())]; ok {
		conn = *v
	} else {
		stream, err := s.streamer.NewStream(ctx, overlay, nil, protocolName, protocolVersion, s.streamName)
		if err != nil {
			return connection{}, err
		}
		conn = connection{
			stream: stream,
		}
		s.conns[[32]byte(overlay.Bytes())] = &conn
	}

	return conn, nil
}

func (p *protocolService) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    p.streamName,
				Handler: p.handler,
			},
		},
	}
}

func (p *protocolService) Broadcast(ctx context.Context, msg []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	group := errgroup.Group{}
	for _, conn := range p.conns {
		conn := conn
		group.Go(func() error { return conn.SendMessage(ctx, msg) })
	}

	if err := group.Wait(); err != nil {
		p.logger.Error(err, "broadcasting L2 message")
	}
}

func (p *protocolService) AddHandler(handler msgHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.handlers = append(p.handlers, handler)
}

func (p *protocolService) RemoveHandler(handler msgHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	handlerValue := reflect.ValueOf(handler).Pointer()

	for i, f := range p.handlers {
		if reflect.ValueOf(f).Pointer() == handlerValue {
			p.handlers = append(p.handlers[:i], p.handlers[i+1:]...)
			return
		}
	}
}

// handler handles incoming messages from all connected peers and calls registered hook functions
func (s *protocolService) handler(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
	for {
		msgSizeBytes := make([]byte, 4)
		n, err := stream.Read(msgSizeBytes)
		if err != nil {
			s.logger.Error(err, "l2 protocol read")
			return err
		}
		if n != 4 {
			return fmt.Errorf("couldn't read 4 bytes for msgSize")
		}
		msgSize := binary.BigEndian.Uint32(msgSizeBytes)
		msg := make([]byte, msgSize)
		var c int
		for c < int(msgSize) {
			n, err := stream.Read(msg[c:])
			if err != nil {
				return err
			}
			c += n
		}
		s.mu.Lock()
		for _, handler := range s.handlers {
			go handler(peer.Address, msg)
		}
		s.mu.Unlock()
	}
}

type IP2pService interface {
	GetProtocol(ctx context.Context, streamName string) (protocol *protocolService)
}

type P2pService struct {
	IP2pService
	mu sync.RWMutex

	streamer  p2p.Streamer
	logger    log.Logger
	protocols map[string]*protocolService // protocolName to its instance
}

func NewP2pService(streamer p2p.Streamer, logger log.Logger) P2pService {
	return P2pService{
		streamer:  streamer,
		logger:    logger,
		protocols: make(map[string]*protocolService),
	}
}

// GetService either creates protocolService or returns initiated one
func (s *P2pService) GetProtocol(ctx context.Context, streamName string) (protocol *protocolService) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.protocols[streamName]; ok {
		protocol = v
	} else {
		p := NewProtocolService(streamName, s.streamer, s.logger)
		protocol = &p
	}

	return protocol
}

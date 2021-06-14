// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hive exposes the hive protocol implementation
// which is the discovery protocol used to inform and be
// informed about other peers in the network. It gossips
// about all peers by default and performs no specific
// prioritization about which peers are gossipped to
// others.
package hive

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"

	"golang.org/x/time/rate"
)

const (
	protocolName    = "hive"
	protocolVersion = "1.0.0"
	peersStreamName = "peers"
	messageTimeout  = 1 * time.Minute // maximum allowed time for a message to be read or written.
	maxBatchSize    = 30
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	limitBurst           = 4 * int(swarm.MaxBins)
	limitRate            = rate.Every(time.Minute)
)

type Service struct {
	streamer        p2p.Streamer
	addressBook     addressbook.GetPutter
	addPeersHandler func(...swarm.Address)
	networkID       uint64
	logger          logging.Logger
	metrics         metrics
	limiter         map[string]*rate.Limiter
	limiterLock     sync.Mutex
}

func New(streamer p2p.Streamer, addressbook addressbook.GetPutter, networkID uint64, logger logging.Logger) *Service {
	return &Service{
		streamer:    streamer,
		logger:      logger,
		addressBook: addressbook,
		networkID:   networkID,
		metrics:     newMetrics(),
		limiter:     make(map[string]*rate.Limiter),
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    peersStreamName,
				Handler: s.peersHandler,
			},
		},
		DisconnectIn:  s.disconnect,
		DisconnectOut: s.disconnect,
	}
}

func (s *Service) BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...swarm.Address) error {
	max := maxBatchSize
	s.metrics.BroadcastPeers.Inc()
	s.metrics.BroadcastPeersPeers.Add(float64(len(peers)))

	for len(peers) > 0 {
		if max > len(peers) {
			max = len(peers)
		}
		if err := s.sendPeers(ctx, addressee, peers[:max]); err != nil {
			return err
		}

		peers = peers[max:]
	}

	return nil
}

func (s *Service) SetAddPeersHandler(h func(addr ...swarm.Address)) {
	s.addPeersHandler = h
}

func (s *Service) sendPeers(ctx context.Context, peer swarm.Address, peers []swarm.Address) (err error) {
	s.metrics.BroadcastPeersSends.Inc()
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, peersStreamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	w, _ := protobuf.NewWriterAndReader(stream)
	var peersRequest pb.Peers
	for _, p := range peers {
		addr, err := s.addressBook.Get(p)
		if err != nil {
			if err == addressbook.ErrNotFound {
				s.logger.Debugf("hive broadcast peers: peer not found in the addressbook. Skipping peer %s", p)
				continue
			}
			return err
		}

		peersRequest.Peers = append(peersRequest.Peers, &pb.BzzAddress{
			Overlay:     addr.Overlay.Bytes(),
			Underlay:    addr.Underlay.Bytes(),
			Signature:   addr.Signature,
			Transaction: addr.Transaction,
		})
	}

	if err := w.WriteMsgWithContext(ctx, &peersRequest); err != nil {
		return fmt.Errorf("write Peers message: %w", err)
	}

	return nil
}

func (s *Service) peersHandler(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
	s.metrics.PeersHandler.Inc()
	_, r := protobuf.NewWriterAndReader(stream)
	ctx, cancel := context.WithTimeout(ctx, messageTimeout)
	defer cancel()
	var peersReq pb.Peers
	if err := r.ReadMsgWithContext(ctx, &peersReq); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("read requestPeers message: %w", err)
	}

	s.metrics.PeersHandlerPeers.Add(float64(len(peersReq.Peers)))

	if err := s.rateLimitPeer(peer.Address, len(peersReq.Peers)); err != nil {
		_ = stream.Reset()
		return err
	}

	// close the stream before processing in order to unblock the sending side
	// fullclose is called async because there is no need to wait for confirmation,
	// but we still want to handle not closed stream from the other side to avoid zombie stream
	go stream.FullClose()

	var peers []swarm.Address
	for _, newPeer := range peersReq.Peers {

		multiUnderlay, err := ma.NewMultiaddrBytes(newPeer.Underlay)
		if err != nil {
			s.logger.Errorf("hive: multi address underlay err: %v", err)
			continue
		}

		bzzAddress := bzz.Address{
			Overlay:     swarm.NewAddress(newPeer.Overlay),
			Underlay:    multiUnderlay,
			Signature:   newPeer.Signature,
			Transaction: newPeer.Transaction,
		}

		err = s.addressBook.Put(bzzAddress.Overlay, bzzAddress)
		if err != nil {
			s.logger.Warningf("skipping peer in response %s: %v", newPeer.String(), err)
			continue
		}

		peers = append(peers, bzzAddress.Overlay)
	}

	if s.addPeersHandler != nil {
		s.addPeersHandler(peers...)
	}

	return nil
}

func (s *Service) rateLimitPeer(peer swarm.Address, count int) error {

	s.limiterLock.Lock()
	defer s.limiterLock.Unlock()

	addr := peer.String()

	limiter, ok := s.limiter[addr]
	if !ok {
		limiter = rate.NewLimiter(limitRate, limitBurst)
		s.limiter[addr] = limiter
	}

	if limiter.AllowN(time.Now(), count) {
		return nil
	}

	return ErrRateLimitExceeded
}

func (s *Service) disconnect(peer p2p.Peer) error {
	s.limiterLock.Lock()
	defer s.limiterLock.Unlock()

	delete(s.limiter, peer.Address.String())

	return nil
}

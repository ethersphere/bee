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
	"github.com/ethersphere/bee/pkg/ratelimit"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	protocolName    = "hive"
	protocolVersion = "1.0.0"
	peersStreamName = "peers"
	messageTimeout  = 1 * time.Minute // maximum allowed time for a message to be read or written.
	maxBatchSize    = 30
)

var (
	limitBurst = 4 * int(swarm.MaxBins)
	limitRate  = time.Minute

	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

type Service struct {
	streamer        p2p.StreamerPinger
	addressBook     addressbook.GetPutter
	addPeersHandler func(...swarm.Address)
	networkID       uint64
	logger          logging.Logger
	metrics         metrics
	inLimiter       *ratelimit.Limiter
	outLimiter      *ratelimit.Limiter
	clearMtx        sync.Mutex
}

func New(streamer p2p.StreamerPinger, addressbook addressbook.GetPutter, networkID uint64, logger logging.Logger) *Service {
	return &Service{
		streamer:    streamer,
		logger:      logger,
		addressBook: addressbook,
		networkID:   networkID,
		metrics:     newMetrics(),
		inLimiter:   ratelimit.New(limitRate, limitBurst),
		outLimiter:  ratelimit.New(limitRate, limitBurst),
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

		// If broadcasting limit is exceeded, return early
		if !s.outLimiter.Allow(addressee.ByteString(), max) {
			return nil
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
			// added this because Recorder (unit test) emits an unnecessary EOF when Close is called
			time.Sleep(time.Millisecond * 50)
			_ = stream.Close()
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

	if !s.inLimiter.Allow(peer.Address.ByteString(), len(peersReq.Peers)) {
		_ = stream.Reset()
		return ErrRateLimitExceeded
	}

	// close the stream before processing in order to unblock the sending side
	// fullclose is called async because there is no need to wait for confirmation,
	// but we still want to handle not closed stream from the other side to avoid zombie stream
	go stream.FullClose()

	go s.checkAndAddPeers(peersReq)

	return nil
}

func (s *Service) disconnect(peer p2p.Peer) error {

	s.clearMtx.Lock()
	defer s.clearMtx.Unlock()

	s.inLimiter.Clear(peer.Address.ByteString())
	s.outLimiter.Clear(peer.Address.ByteString())

	return nil
}

const checkPeersTimeout = time.Minute * 15

func (s *Service) checkAndAddPeers(peers pb.Peers) {
	ctx, cancel := context.WithTimeout(context.Background(), checkPeersTimeout)
	defer cancel()

	sem := make(chan struct{}, 5)
	var peersToAdd []swarm.Address
	mtx := sync.Mutex{}
	wg := sync.WaitGroup{}

	for i := range peers.Peers {
		sem <- struct{}{}

		wg.Add(1)
		go func(idx int) {
			defer func() {
				<-sem
				wg.Done()
			}()

			newPeer := peers.Peers[idx]

			multiUnderlay, err := ma.NewMultiaddrBytes(newPeer.Underlay)
			if err != nil {
				s.logger.Errorf("hive: multi address underlay err: %v", err)
				return
			}

			// check if the underlay is usable by doing a raw ping using libp2p
			_, err = s.streamer.Ping(ctx, multiUnderlay)
			if err != nil {
				s.metrics.UnreachablePeers.Inc()
				s.logger.Warningf("hive: multi address underlay %s not reachable err: %w", multiUnderlay, err)
				return
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
				return
			}

			mtx.Lock()
			peersToAdd = append(peersToAdd, bzzAddress.Overlay)
			mtx.Unlock()
		}(i)
	}

	wg.Wait()

	if s.addPeersHandler != nil && len(peersToAdd) > 0 {
		s.addPeersHandler(peersToAdd...)
	}
}

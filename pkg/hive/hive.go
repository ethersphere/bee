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
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/ratelimit"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "hive"

const (
	protocolName           = "hive"
	protocolVersion        = "1.1.0"
	peersStreamName        = "peers"
	messageTimeout         = 1 * time.Minute // maximum allowed time for a message to be read or written.
	maxBatchSize           = 30
	pingTimeout            = time.Second * 15 // time to wait for ping to succeed
	batchValidationTimeout = 5 * time.Minute  // prevent lock contention on peer validation
)

var (
	limitBurst = 4 * int(swarm.MaxBins)
	limitRate  = time.Minute

	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

type Service struct {
	streamer          p2p.StreamerPinger
	addressBook       addressbook.GetPutter
	addPeersHandler   func(...swarm.Address)
	networkID         uint64
	logger            log.Logger
	metrics           metrics
	inLimiter         *ratelimit.Limiter
	outLimiter        *ratelimit.Limiter
	quit              chan struct{}
	wg                sync.WaitGroup
	peersChan         chan pb.Peers
	sem               *semaphore.Weighted
	bootnode          bool
	allowPrivateCIDRs bool
}

func New(streamer p2p.StreamerPinger, addressbook addressbook.GetPutter, networkID uint64, bootnode bool, allowPrivateCIDRs bool, logger log.Logger) *Service {
	svc := &Service{
		streamer:          streamer,
		logger:            logger.WithName(loggerName).Register(),
		addressBook:       addressbook,
		networkID:         networkID,
		metrics:           newMetrics(),
		inLimiter:         ratelimit.New(limitRate, limitBurst),
		outLimiter:        ratelimit.New(limitRate, limitBurst),
		quit:              make(chan struct{}),
		peersChan:         make(chan pb.Peers),
		sem:               semaphore.NewWeighted(int64(swarm.MaxBins)),
		bootnode:          bootnode,
		allowPrivateCIDRs: allowPrivateCIDRs,
	}

	if !bootnode {
		svc.startCheckPeersHandler()
	}

	return svc
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

var ErrShutdownInProgress = errors.New("shutdown in progress")

func (s *Service) BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...swarm.Address) error {
	maxSize := maxBatchSize
	s.metrics.BroadcastPeers.Inc()
	s.metrics.BroadcastPeersPeers.Add(float64(len(peers)))

	for len(peers) > 0 {
		if maxSize > len(peers) {
			maxSize = len(peers)
		}

		// If broadcasting limit is exceeded, return early
		if !s.outLimiter.Allow(addressee.ByteString(), maxSize) {
			return nil
		}

		select {
		case <-s.quit:
			return ErrShutdownInProgress
		default:
		}

		if err := s.sendPeers(ctx, addressee, peers[:maxSize]); err != nil {
			return err
		}

		peers = peers[maxSize:]
	}

	return nil
}

func (s *Service) SetAddPeersHandler(h func(addr ...swarm.Address)) {
	s.addPeersHandler = h
}

func (s *Service) Close() error {
	close(s.quit)

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.wg.Wait()
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(time.Second * 5):
		return errors.New("hive: waited 5 seconds to close active goroutines")
	}
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
			_ = stream.Close()
		}
	}()
	w, _ := protobuf.NewWriterAndReader(stream)
	var peersRequest pb.Peers
	for _, p := range peers {
		addr, err := s.addressBook.Get(p)
		if err != nil {
			if errors.Is(err, addressbook.ErrNotFound) {
				s.logger.Debug("broadcast peers; peer not found in the addressbook, skipping...", "peer_address", p)
				continue
			}
			return err
		}

		if !s.allowPrivateCIDRs && manet.IsPrivateAddr(addr.Underlay) {
			continue // Don't advertise private CIDRs to the public network.
		}

		peersRequest.Peers = append(peersRequest.Peers, &pb.BzzAddress{
			Overlay:   addr.Overlay.Bytes(),
			Underlay:  addr.Underlay.Bytes(),
			Signature: addr.Signature,
			Nonce:     addr.Nonce,
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

	if s.bootnode {
		return nil
	}

	select {
	case s.peersChan <- peersReq:
	case <-s.quit:
		return errors.New("failed to process peers, shutting down hive")
	}

	return nil
}

func (s *Service) disconnect(peer p2p.Peer) error {
	s.inLimiter.Clear(peer.Address.ByteString())
	s.outLimiter.Clear(peer.Address.ByteString())
	return nil
}

func (s *Service) startCheckPeersHandler() {
	ctx, cancel := context.WithCancel(context.Background())
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.quit
		cancel()
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case newPeers := <-s.peersChan:
				s.wg.Add(1)
				go func() {
					defer s.wg.Done()
					cctx, cancel := context.WithTimeout(ctx, batchValidationTimeout)
					defer cancel()
					s.checkAndAddPeers(cctx, newPeers)
				}()
			}
		}
	}()
}

func (s *Service) checkAndAddPeers(ctx context.Context, peers pb.Peers) {
	var peersToAdd []swarm.Address
	mtx := sync.Mutex{}
	wg := sync.WaitGroup{}

	addPeer := func(newPeer *pb.BzzAddress, multiUnderlay ma.Multiaddr) {
		err := s.sem.Acquire(ctx, 1)
		if err != nil {
			return
		}

		wg.Add(1)
		go func() {
			s.metrics.PeerConnectAttempts.Inc()

			defer func() {
				s.sem.Release(1)
				wg.Done()
			}()

			ctx, cancel := context.WithTimeout(ctx, pingTimeout)
			defer cancel()

			start := time.Now()

			// check if the underlay is usable by doing a raw ping using libp2p
			if _, err := s.streamer.Ping(ctx, multiUnderlay); err != nil {
				s.metrics.PingFailureTime.Observe(time.Since(start).Seconds())
				s.metrics.UnreachablePeers.Inc()
				s.logger.Debug("unreachable peer underlay", "peer_address", hex.EncodeToString(newPeer.Overlay), "underlay", multiUnderlay)
				return
			}
			s.metrics.PingTime.Observe(time.Since(start).Seconds())

			s.metrics.ReachablePeers.Inc()

			bzzAddress := bzz.Address{
				Overlay:   swarm.NewAddress(newPeer.Overlay),
				Underlay:  multiUnderlay,
				Signature: newPeer.Signature,
				Nonce:     newPeer.Nonce,
			}

			err := s.addressBook.Put(bzzAddress.Overlay, bzzAddress)
			if err != nil {
				s.metrics.StorePeerErr.Inc()
				s.logger.Warning("skipping peer in response", "peer_address", newPeer.String(), "error", err)
				return
			}

			mtx.Lock()
			peersToAdd = append(peersToAdd, bzzAddress.Overlay)
			mtx.Unlock()
		}()
	}

	for _, p := range peers.Peers {

		multiUnderlay, err := ma.NewMultiaddrBytes(p.Underlay)
		if err != nil {
			s.metrics.PeerUnderlayErr.Inc()
			s.logger.Debug("multi address underlay", "error", err)
			continue
		}

		// if peer exists already in the addressBook
		// and if the underlays match, skip
		addr, err := s.addressBook.Get(swarm.NewAddress(p.Overlay))
		if err == nil && addr.Underlay.Equal(multiUnderlay) {
			continue
		}

		// add peer does not exist in the addressbook
		addPeer(p, multiUnderlay)
	}
	wg.Wait()

	if s.addPeersHandler != nil && len(peersToAdd) > 0 {
		s.addPeersHandler(peersToAdd...)
	}
}

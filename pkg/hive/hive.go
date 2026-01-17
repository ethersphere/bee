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
	protocolName    = "hive"
	protocolVersion = "1.1.0"
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
	streamer          p2p.Streamer
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
	overlay           swarm.Address
}

func New(streamer p2p.Streamer, addressbook addressbook.GetPutter, networkID uint64, bootnode bool, allowPrivateCIDRs bool, overlay swarm.Address, logger log.Logger) *Service {
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
		overlay:           overlay,
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
		if p.Equal(s.overlay) {
			s.logger.Debug("skipping self-address in broadcast", "overlay", p.String())
			continue
		}

		addr, err := s.addressBook.Get(p)
		if err != nil {
			if errors.Is(err, addressbook.ErrNotFound) {
				s.logger.Debug("broadcast peers; peer not found in the addressbook, skipping...", "peer_address", p)
				continue
			}
			return err
		}

		advertisableUnderlays := s.filterAdvertisableUnderlays(addr.Underlays)
		if len(advertisableUnderlays) == 0 {
			s.logger.Debug("skipping peers: no advertisable underlays", "peer_address", p)
			continue
		}

		peersRequest.Peers = append(peersRequest.Peers, &pb.BzzAddress{
			Overlay:   addr.Overlay.Bytes(),
			Underlay:  bzz.SerializeUnderlays(advertisableUnderlays),
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
	s.wg.Go(func() {
		<-s.quit
		cancel()
	})

	s.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case newPeers := <-s.peersChan:
				s.wg.Go(func() {
					s.checkAndAddPeers(newPeers)
				})
			}
		}
	})
}

func (s *Service) checkAndAddPeers(peers pb.Peers) {
	peersToAdd := make([]swarm.Address, 0, len(peers.Peers))

	for _, p := range peers.Peers {
		multiUnderlays, err := bzz.DeserializeUnderlays(p.Underlay)
		if err != nil {
			s.metrics.PeerUnderlayErr.Inc()
			s.logger.Debug("multi address underlay", "error", err)
			continue
		}

		if len(multiUnderlays) == 0 {
			s.logger.Debug("check and add peers, no underlays", "overlay", swarm.NewAddress(p.Overlay).String())
			continue // no underlays sent
		}

		// if peer exists already in the addressBook
		// and if the underlays match, skip
		addr, err := s.addressBook.Get(swarm.NewAddress(p.Overlay))
		if err == nil && bzz.AreUnderlaysEqual(addr.Underlays, multiUnderlays) {
			continue
		}

		overlayAddr := swarm.NewAddress(p.Overlay)

		if overlayAddr.Equal(s.overlay) {
			s.logger.Debug("skipping self-address in peer list", "overlay", overlayAddr.String())
			continue
		}

		bzzAddress := bzz.Address{
			Overlay:   overlayAddr,
			Underlays: multiUnderlays,
			Signature: p.Signature,
			Nonce:     p.Nonce,
		}

		if err := s.addressBook.Put(bzzAddress.Overlay, bzzAddress); err != nil {
			s.metrics.StorePeerErr.Inc()
			s.logger.Warning("put peer in addressbook", "peer_address", p.String(), "error", err)
			continue
		}

		peersToAdd = append(peersToAdd, bzzAddress.Overlay)
	}

	if s.addPeersHandler != nil && len(peersToAdd) > 0 {
		s.addPeersHandler(peersToAdd...)
	}
}

// filterAdvertisableUnderlays returns underlay addresses that can be advertised
// based on the allowPrivateCIDRs setting. If allowPrivateCIDRs is false,
// only public addresses are returned.
func (s *Service) filterAdvertisableUnderlays(underlays []ma.Multiaddr) []ma.Multiaddr {
	if s.allowPrivateCIDRs {
		return underlays
	}

	var publicUnderlays []ma.Multiaddr
	for _, u := range underlays {
		if !manet.IsPrivateAddr(u) {
			publicUnderlays = append(publicUnderlays, u)
		}
	}

	return publicUnderlays
}

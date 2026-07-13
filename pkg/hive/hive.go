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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/ratelimit"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"golang.org/x/sync/semaphore"
)

// ChequebookStorer persists the overlay→chequebook mapping. Put holds its
// internal mutex for the duration of writeAddressbook, so registry and
// addressbook writes are atomic with respect to concurrent ingestions.
type ChequebookStorer interface {
	Put(peer swarm.Address, chequebook common.Address, timestamp int64, source bzz.TimestampSource, writeAddressbook func() error) error
}

// loggerName is the tree path name of the logger for this package.
const loggerName = "hive"

const (
	protocolName    = "hive"
	protocolVersion = "2.0.0"
	peersStreamName = "peers"
	messageTimeout  = 1 * time.Minute // maximum allowed time for a message to be read or written.
	maxBatchSize    = 30
)

var (
	limitBurst = 4 * int(swarm.MaxBins)
	limitRate  = time.Minute

	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// Options configures hive.Service at construction. Chequebook fields are
// optional: a nil ChequebookVerifier disables the verification gate (and
// records without a chequebook are accepted); a nil ChequebookStorer means
// no overlay→chequebook persistence — addressbook writes happen directly.
type Options struct {
	BootnodeMode       bool
	AllowPrivateCIDRs  bool
	ChequebookVerifier chequebook.Verifier
	ChequebookStorer   ChequebookStorer
}

type Service struct {
	streamer          p2p.Streamer
	addressBook       addressbook.GetPutUpdater
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
	now               func() time.Time

	// nil verifier disables the check entirely; when set, records without a
	// chequebook are dropped.
	chequebookVerifier chequebook.Verifier
	chequebookStorer   ChequebookStorer
}

func New(streamer p2p.Streamer, addressbook addressbook.GetPutUpdater, networkID uint64, overlay swarm.Address, logger log.Logger, o Options) *Service {
	svc := &Service{
		streamer:           streamer,
		logger:             logger.WithName(loggerName).Register(),
		addressBook:        addressbook,
		networkID:          networkID,
		metrics:            newMetrics(),
		inLimiter:          ratelimit.New(limitRate, limitBurst),
		outLimiter:         ratelimit.New(limitRate, limitBurst),
		quit:               make(chan struct{}),
		peersChan:          make(chan pb.Peers),
		sem:                semaphore.NewWeighted(int64(swarm.MaxBins)),
		bootnode:           o.BootnodeMode,
		allowPrivateCIDRs:  o.AllowPrivateCIDRs,
		overlay:            overlay,
		now:                time.Now,
		chequebookVerifier: o.ChequebookVerifier,
		chequebookStorer:   o.ChequebookStorer,
	}

	if !o.BootnodeMode {
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

		addr, _, err := s.addressBook.Get(p)
		if err != nil {
			if errors.Is(err, addressbook.ErrNotFound) {
				s.logger.Debug("broadcast peers; peer not found in the addressbook, skipping...", "peer_address", p)
				continue
			}
			return err
		}

		// Skip legacy pre-timestamp records — receivers on the new protocol
		// reject Timestamp==0 on the wire, so gossiping them is wasted noise.
		if addr.Timestamp == 0 {
			s.metrics.LegacyRecordSkipped.Inc()
			continue
		}

		// Skip peers with no advertisable underlays — no point gossiping
		// a peer that receivers won't be able to connect to.
		if len(s.filterAdvertisableUnderlays(addr.Underlays)) == 0 {
			s.logger.Debug("skipping peers: no advertisable underlays", "peer_address", p)
			continue
		}

		underlayBytes, err := bzz.SerializeUnderlays(addr.Underlays)
		if err != nil {
			s.logger.Warning("sendPeers: failed to serialize underlays", "peer_address", p, "error", err)
			continue
		}

		// Send full underlays - the signature covers the complete set,
		// so the receiver needs it for verification.
		peersRequest.Peers = append(peersRequest.Peers, &pb.BzzAddress{
			Overlay:           addr.Overlay.Bytes(),
			Underlay:          underlayBytes,
			Signature:         addr.Signature,
			Nonce:             addr.Nonce,
			Timestamp:         addr.Timestamp,
			ChequebookAddress: addr.ChequebookAddress.Bytes(),
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
					s.checkAndAddPeers(ctx, newPeers)
				})
			}
		}
	})
}

func (s *Service) checkAndAddPeers(ctx context.Context, peers pb.Peers) {
	peersToAdd := make([]swarm.Address, 0, len(peers.Peers))

	for _, p := range peers.Peers {
		if p == nil {
			s.logger.Debug("nil peer entry in Peers message, skipping")
			continue
		}

		multiUnderlays, err := bzz.DeserializeUnderlays(p.Underlay)
		if err != nil {
			s.metrics.PeerUnderlayErr.Inc()
			switch {
			case errors.Is(err, bzz.ErrUnderlayByteSizeExceeded):
				s.metrics.UnderlayByteSizeExceeded.Inc()
				s.logger.Warning("checkAndAddPeers: dropping peer with oversized underlay", "size", len(p.Underlay))
			case errors.Is(err, bzz.ErrUnderlayCountExceeded):
				s.metrics.UnderlayCountExceeded.Inc()
				s.logger.Warning("checkAndAddPeers: dropping peer with too many underlays", "size", len(p.Underlay))
			default:
				s.logger.Debug("multi address underlay", "error", err)
			}
			continue
		}

		if len(multiUnderlays) == 0 {
			s.logger.Debug("check and add peers, no underlays", "overlay", swarm.NewAddress(p.Overlay).String())
			continue // no underlays sent
		}

		overlayAddr := swarm.NewAddress(p.Overlay)

		if overlayAddr.Equal(s.overlay) {
			s.logger.Debug("skipping self-address in peer list", "overlay", overlayAddr.String())
			continue
		}

		bzzAddress, err := bzz.ParseAddress(p.Underlay, p.Overlay, p.Signature, p.Nonce, p.Timestamp, s.networkID, p.ChequebookAddress)
		if err != nil {
			s.logger.Debug("hive gossip: invalid address record", "overlay", overlayAddr.String(), "error", err)
			continue
		}

		existing, pastVerified, err := s.addressBook.Get(overlayAddr)
		if err != nil && !errors.Is(err, addressbook.ErrNotFound) {
			s.logger.Debug("hive gossip: addressbook lookup failed", "overlay", overlayAddr.String(), "error", err)
			continue
		}

		if err := bzz.CheckTimestamp(bzzAddress.Timestamp, existing, bzz.TimestampSourceGossip, s.now()); err != nil {
			// A peer re-presenting a record we already hold has nothing new to
			// store, but it is still a sighting: peers mint their bzz.Address
			// once and gossip it unchanged for their whole uptime. Refresh
			// last-seen so peers we keep hearing about, but never dial, do not
			// look stale to the pruner. Both errors imply a known peer, since
			// an unknown one short-circuits inside CheckTimestamp.
			if errors.Is(err, bzz.ErrTimestampStale) || errors.Is(err, bzz.ErrTimestampTooSoon) {
				if err := s.addressBook.UpdateLastSeen(overlayAddr); err != nil {
					s.logger.Debug("hive gossip: update last seen", "overlay", overlayAddr.String(), "error", err)
				}
			}
			s.bumpTimestampMetric(err)
			s.logger.Debug("hive gossip: timestamp validation failed", "overlay", overlayAddr.String(), "error", err)
			continue
		}

		if s.chequebookVerifier != nil {
			if (bzzAddress.ChequebookAddress == common.Address{}) {
				s.metrics.ChequebookVerification.WithLabelValues("missing").Inc()
				s.logger.Debug("hive gossip: rejecting record without chequebook", "overlay", overlayAddr.String())
				continue
			}
			pairVerified := pastVerified && existing != nil && existing.ChequebookAddress == bzzAddress.ChequebookAddress
			peerEth := common.BytesToAddress(bzzAddress.EthereumAddress)
			if err := s.chequebookVerifier.Verify(ctx, bzzAddress.ChequebookAddress, peerEth, overlayAddr, pairVerified); err != nil {
				s.metrics.ChequebookVerification.WithLabelValues(chequebook.VerifyErrorLabel(err)).Inc()
				s.logger.Debug("hive gossip: chequebook verification failed", "overlay", overlayAddr.String(), "error", err)
				continue
			}
			s.metrics.ChequebookVerification.WithLabelValues("success").Inc()
		}

		if s.chequebookStorer != nil {
			// The addressbook write is passed as a callback so it runs under the
			// registry's mutex, keeping the in-memory registry and on-disk addressbook
			// in sync against concurrent gossip ingestions for the same peer.
			if err := s.chequebookStorer.Put(
				overlayAddr, bzzAddress.ChequebookAddress, bzzAddress.Timestamp, bzz.TimestampSourceGossip,
				func() error { return s.addressBook.Put(bzzAddress.Overlay, *bzzAddress, true) },
			); err != nil {
				if s.bumpTimestampMetric(err) {
					s.logger.Debug("hive gossip: chequebook registry rejected update", "overlay", overlayAddr.String(), "error", err)
					continue
				}
				s.metrics.StorePeerErr.Inc()
				s.logger.Warning("put peer in addressbook", "peer_address", p.String(), "error", err)
				continue
			}
		} else {
			if err := s.addressBook.Put(bzzAddress.Overlay, *bzzAddress, false); err != nil {
				s.metrics.StorePeerErr.Inc()
				s.logger.Warning("put peer in addressbook", "peer_address", p.String(), "error", err)
				continue
			}
		}

		peersToAdd = append(peersToAdd, bzzAddress.Overlay)
	}

	if s.addPeersHandler != nil && len(peersToAdd) > 0 {
		s.addPeersHandler(peersToAdd...)
	}
}

// bumpTimestampMetric records the rejection and returns whether err was a timestamp sentinel.
func (s *Service) bumpTimestampMetric(err error) bool {
	reason, ok := bzz.TimestampErrorLabel(err)
	if ok {
		s.metrics.TimestampRejected.WithLabelValues(reason).Inc()
	}
	return ok
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

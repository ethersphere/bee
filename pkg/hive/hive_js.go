//go:build js
// +build js

package hive

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/ratelimit"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	manet "github.com/multiformats/go-multiaddr/net"
	"golang.org/x/sync/semaphore"
)

type Service struct {
	streamer          p2p.StreamerPinger
	addressBook       addressbook.GetPutter
	addPeersHandler   func(...swarm.Address)
	networkID         uint64
	logger            log.Logger
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

func (s *Service) BroadcastPeers(ctx context.Context, addressee swarm.Address, peers ...swarm.Address) error {
	maxSize := maxBatchSize

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

func (s *Service) sendPeers(ctx context.Context, peer swarm.Address, peers []swarm.Address) (err error) {

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
			// continue // Don't advertise private CIDRs to the public network.
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

	_, r := protobuf.NewWriterAndReader(stream)
	ctx, cancel := context.WithTimeout(ctx, messageTimeout)
	defer cancel()
	var peersReq pb.Peers
	if err := r.ReadMsgWithContext(ctx, &peersReq); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("read requestPeers message: %w", err)
	}

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

func (s *Service) checkAndAddPeers(ctx context.Context, peers pb.Peers) {
	var peersToAdd []swarm.Address
	mtx := sync.Mutex{}
	wg := sync.WaitGroup{}

	addPeer := func(newPeer *pb.BzzAddress, multiUnderlay ma.Multiaddr) {
		wsProto := false
		ma.ForEach(multiUnderlay, func(c ma.Component) bool {
			if c.Protocol().Name == "ws" {
				wsProto = true
				return false
			}
			return true
		})

		if !wsProto {
			s.logger.Debug("skipping non-websocket peer", "peer_address", hex.EncodeToString(newPeer.Overlay), "underlay", multiUnderlay)
			return
		}

		err := s.sem.Acquire(ctx, 1)
		if err != nil {
			return
		}

		wg.Add(1)
		go func() {

			defer func() {
				s.sem.Release(1)
				wg.Done()
			}()

			ctx, cancel := context.WithTimeout(ctx, pingTimeout)
			defer cancel()

			// check if the underlay is usable by doing a raw ping using libp2p
			if _, err := s.streamer.Ping(ctx, multiUnderlay); err != nil {

				s.logger.Debug("unreachable peer underlay", "peer_address", hex.EncodeToString(newPeer.Overlay), "underlay", multiUnderlay)
				return
			}

			bzzAddress := bzz.Address{
				Overlay:   swarm.NewAddress(newPeer.Overlay),
				Underlay:  multiUnderlay,
				Signature: newPeer.Signature,
				Nonce:     newPeer.Nonce,
			}

			err := s.addressBook.Put(bzzAddress.Overlay, bzzAddress)
			if err != nil {
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

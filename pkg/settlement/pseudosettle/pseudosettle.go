// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/settlement"
	pb "github.com/ethersphere/bee/pkg/settlement/pseudosettle/pb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName    = "pseudosettle"
	protocolVersion = "1.0.0"
	streamName      = "pseudosettle"
)

type Service struct {
	streamer p2p.Streamer
	logger   logging.Logger
	store    storage.StateStorer
	observer settlement.PaymentObserver
	metrics  metrics
}

func New(streamer p2p.Streamer, logger logging.Logger, store storage.StateStorer) *Service {
	return &Service{
		streamer: o.Streamer,
		logger:   o.Logger,
		metrics:  newMetrics(),
		store:    store,
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Handler: s.handler,
			},
		},
	}
}

func totalReceivedKey(peer swarm.Address) string {
	return fmt.Sprintf("pseudosettle_total_received_%v", peer)
}

func totalSentKey(peer swarm.Address) string {
	return fmt.Sprintf("pseudosettle_total_sent_%v", peer)
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	r := protobuf.NewReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	var req pb.Payment
	if err := r.ReadMsg(&req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	s.metrics.TotalReceivedPseudoSettlements.Add(float64(req.Amount))
	s.logger.Tracef("received payment message from peer %v of %d", p.Address, req.Amount)

	totalReceived, err := s.TotalReceived(p.Address)
	if err != nil {
		return err
	}

	err = s.store.Put(totalReceivedKey(p.Address), totalReceived+req.Amount)
	if err != nil {
		return err
	}

	return s.observer.NotifyPayment(p.Address, req.Amount)
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount uint64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	s.logger.Tracef("sending payment message to peer %v of %d", peer, amount)
	w := protobuf.NewWriter(stream)
	err = w.WriteMsgWithContext(ctx, &pb.Payment{
		Amount: amount,
	})
	if err != nil {
		return err
	}
	totalSent, err := s.TotalSent(peer)
	if err != nil {
		return err
	}
	err = s.store.Put(totalSentKey(peer), totalSent+amount)
	if err != nil {
		return err
	}
	s.metrics.TotalSentPseudoSettlements.Add(float64(amount))
	return nil
}

// SetPaymentObserver sets the payment observer which will be notified of incoming payments
func (s *Service) SetPaymentObserver(observer settlement.PaymentObserver) {
	s.observer = observer
}

// TotalSent returns the total amount sent to a peer
func (s *Service) TotalSent(peer swarm.Address) (totalSent uint64, err error) {
	key := fmt.Sprintf("pseudosettle_total_sent_%v", peer)
	err = s.store.Get(key, &totalSent)
	if err != nil {
		if err == storage.ErrNotFound {
			return 0, nil
		} else {
			return 0, err
		}
	}
	return totalSent, nil
}

// TotalReceived returns the total amount received from a peer
func (s *Service) TotalReceived(peer swarm.Address) (totalReceived uint64, err error) {
	key := fmt.Sprintf("pseudosettle_total_received_%v", peer)
	err = s.store.Get(key, &totalReceived)
	if err != nil {
		if err == storage.ErrNotFound {
			return 0, nil
		} else {
			return 0, err
		}
	}
	return totalReceived, nil
}

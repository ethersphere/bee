// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing

import (
	"context"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pricing/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName    = "pricing"
	protocolVersion = "1.0.0"
	streamName      = "pricing"
)

var _ Interface = (*Service)(nil)

type Interface interface {
	AnnouncePaymentThreshold(ctx context.Context, peer swarm.Address, paymentThreshold uint64) error
}

type PaymentThresholdObserver interface {
	NotifyPaymentThreshold(peer swarm.Address, paymentThreshold uint64) error
}

type Service struct {
	streamer                 p2p.Streamer
	logger                   logging.Logger
	paymentThresholdObserver PaymentThresholdObserver
}

func New(streamer p2p.Streamer, logger logging.Logger) *Service {
	return &Service{
		streamer: streamer,
		logger:   logger,
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

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	r := protobuf.NewReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	var req pb.AnnouncePaymentThreshold
	if err := r.ReadMsg(&req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	s.logger.Tracef("received payment threshold announcement from peer %v of %d", p.Address, req.PaymentThreshold)

	return s.paymentThresholdObserver.NotifyPaymentThreshold(p.Address, req.PaymentThreshold)
}

// AnnouncePaymentThreshold announces the payment threshold to per
func (s *Service) AnnouncePaymentThreshold(ctx context.Context, peer swarm.Address, paymentThreshold uint64) error {
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

	s.logger.Tracef("sending payment threshold announcement to peer %v of %d", peer, paymentThreshold)
	w := protobuf.NewWriter(stream)
	err = w.WriteMsgWithContext(ctx, &pb.AnnouncePaymentThreshold{
		PaymentThreshold: paymentThreshold,
	})

	return err
}

func (s *Service) SetPaymentThresholdObserver(observer PaymentThresholdObserver) {
	s.paymentThresholdObserver = observer
}

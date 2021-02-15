// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/pricer"
	"github.com/ethersphere/bee/pkg/pricing/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName    = "pricing"
	protocolVersion = "1.0.0"
	streamName      = "pricing"
)

var _ Interface = (*Service)(nil)

// Interface is the main interface of the pricing protocol
type Interface interface {
	AnnouncePaymentThreshold(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error
	AnnouncePaymentThresholdAndPriceTable(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error
}

// PriceTableObserver is used for being notified of price table updates
type PriceTableObserver interface {
	NotifyPriceTable(peer swarm.Address, priceTable []uint64) error
}

// PaymentThresholdObserver is used for being notified of payment threshold updates
type PaymentThresholdObserver interface {
	NotifyPaymentThreshold(peer swarm.Address, paymentThreshold *big.Int) error
}

type Service struct {
	streamer                 p2p.Streamer
	logger                   logging.Logger
	paymentThreshold         *big.Int
	pricer                   pricer.Interface
	paymentThresholdObserver PaymentThresholdObserver
	priceTableObserver       PriceTableObserver
}

func New(streamer p2p.Streamer, logger logging.Logger, paymentThreshold *big.Int, pricer pricer.Interface) *Service {
	return &Service{
		streamer:         streamer,
		logger:           logger,
		pricer:           pricer,
		paymentThreshold: paymentThreshold,
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
		ConnectIn:  s.init,
		ConnectOut: s.init,
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
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		s.logger.Debugf("could not receive payment threshold and/or price table announcement from peer %v", p.Address)
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	paymentThreshold := big.NewInt(0).SetBytes(req.PaymentThreshold)
	s.logger.Tracef("received payment threshold announcement from peer %v of %d", p.Address, paymentThreshold)

	if req.ProximityPrice != nil {
		s.logger.Tracef("received pricetable announcement from peer %v of %v", p.Address, req.ProximityPrice)
		err = s.priceTableObserver.NotifyPriceTable(p.Address, req.ProximityPrice)
		if err != nil {
			s.logger.Debugf("error receiving pricetable from peer %v: %w", p.Address, err)
			s.logger.Errorf("error receiving pricetable from peer %v: %w", p.Address, err)
		}
	}

	if paymentThreshold.Cmp(big.NewInt(0)) == 0 {
		return err
	}
	return s.paymentThresholdObserver.NotifyPaymentThreshold(p.Address, paymentThreshold)
}

func (s *Service) init(ctx context.Context, p p2p.Peer) error {
	err := s.AnnouncePaymentThresholdAndPriceTable(ctx, p.Address, s.paymentThreshold)
	if err != nil {
		s.logger.Warningf("could not send payment threshold announcement to peer %v", p.Address)
	}
	return err
}

// AnnouncePaymentThreshold announces the payment threshold to per
func (s *Service) AnnouncePaymentThreshold(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error {
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
		PaymentThreshold: paymentThreshold.Bytes(),
	})

	return err
}

// AnnouncePaymentThresholdAndPriceTable announces own payment threshold and pricetable to peer
func (s *Service) AnnouncePaymentThresholdAndPriceTable(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error {
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
		PaymentThreshold: paymentThreshold.Bytes(),
		ProximityPrice:   s.pricer.PriceTable(),
	})

	return err
}

// SetPaymentThresholdObserver sets the PaymentThresholdObserver to be used when receiving a new payment threshold
func (s *Service) SetPaymentThresholdObserver(observer PaymentThresholdObserver) {
	s.paymentThresholdObserver = observer
}

// SetPriceTableObserver sets the PriceTableObserver to be used when receiving a new pricetable
func (s *Service) SetPriceTableObserver(observer PriceTableObserver) {
	s.priceTableObserver = observer
}

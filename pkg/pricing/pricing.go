// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pricing/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pricing"

const (
	protocolName    = "pricing"
	protocolVersion = "1.0.0"
	streamName      = "pricing"
)

var (
	// ErrThresholdTooLow says that the proposed payment threshold is too low for even a single reserve.
	ErrThresholdTooLow = errors.New("threshold too low")
)

var _ Interface = (*Service)(nil)

// Interface is the main interface of the pricing protocol
type Interface interface {
	AnnouncePaymentThreshold(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error
}

// PaymentThresholdObserver is used for being notified of payment threshold updates
type PaymentThresholdObserver interface {
	NotifyPaymentThreshold(peer swarm.Address, paymentThreshold *big.Int) error
}

type Service struct {
	streamer                 p2p.Streamer
	logger                   log.Logger
	paymentThreshold         *big.Int
	lightPaymentThreshold    *big.Int
	minPaymentThreshold      *big.Int
	paymentThresholdObserver PaymentThresholdObserver
}

func New(streamer p2p.Streamer, logger log.Logger, paymentThreshold, lightPaymentThreshold, minThreshold *big.Int) *Service {
	return &Service{
		streamer:              streamer,
		logger:                logger.WithName(loggerName).Register(),
		paymentThreshold:      paymentThreshold,
		lightPaymentThreshold: lightPaymentThreshold,
		minPaymentThreshold:   minThreshold,
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
	loggerV1 := s.logger.V(1).Register()

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
		s.logger.Debug("could not receive payment threshold and/or price table announcement from peer", "peer_address", p.Address)
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	paymentThreshold := big.NewInt(0).SetBytes(req.PaymentThreshold)
	loggerV1.Debug("received payment threshold announcement from peer", "peer_address", p.Address, "payment_threshold", paymentThreshold)

	if paymentThreshold.Cmp(s.minPaymentThreshold) < 0 {
		loggerV1.Debug("payment threshold from peer too small, need at least min payment threshold", "peer_address", p.Address, "payment_threshold", paymentThreshold, "min_payment_threshold", s.minPaymentThreshold)
		return p2p.NewDisconnectError(ErrThresholdTooLow)
	}

	if paymentThreshold.Cmp(big.NewInt(0)) == 0 {
		return err
	}
	return s.paymentThresholdObserver.NotifyPaymentThreshold(p.Address, paymentThreshold)
}

func (s *Service) init(ctx context.Context, p p2p.Peer) error {

	threshold := s.paymentThreshold
	if !p.FullNode {
		threshold = s.lightPaymentThreshold
	}

	err := s.AnnouncePaymentThreshold(ctx, p.Address, threshold)
	if err != nil {
		s.logger.Warning("could not send payment threshold announcement to peer", "peer_address", p.Address)
	}
	return err
}

// AnnouncePaymentThreshold announces the payment threshold to per
func (s *Service) AnnouncePaymentThreshold(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error {
	loggerV1 := s.logger.V(1).Register()

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
			stream.FullClose()
		}
	}()

	loggerV1.Debug("sending payment threshold announcement to peer", "peer_address", peer, "payment_threshold", paymentThreshold)
	w := protobuf.NewWriter(stream)
	err = w.WriteMsgWithContext(ctx, &pb.AnnouncePaymentThreshold{
		PaymentThreshold: paymentThreshold.Bytes(),
	})

	return err
}

// SetPaymentThresholdObserver sets the PaymentThresholdObserver to be used when receiving a new payment threshold
func (s *Service) SetPaymentThresholdObserver(observer PaymentThresholdObserver) {
	s.paymentThresholdObserver = observer
}

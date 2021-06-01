// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/exchange"
	swap "github.com/ethersphere/bee/pkg/settlement/swap/headers"
	"github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName    = "swap"
	protocolVersion = "1.0.0"
	streamName      = "swap" // stream for cheques
	initStreamName  = "init" // stream for handshake
)

var (
	ErrNegotiateRate      = errors.New("exchange rates mismatch")
	ErrNegotiateDeduction = errors.New("deduction values mismatch")
	ErrHaveDeduction      = errors.New("received deduction not zero")
)

type SendChequeFunc chequebook.SendChequeFunc

type IssueFunc func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error)

// (context.Context, common.Address, *big.Int, chequebook.SendChequeFunc) (*big.Int, error)

// Interface is the main interface to send messages over swap protocol.
type Interface interface {
	// EmitCheque sends a signed cheque to a peer.
	EmitCheque(ctx context.Context, peer swarm.Address, beneficiary common.Address, amount *big.Int, issue IssueFunc) (balance *big.Int, err error)
}

// Swap is the interface the settlement layer should implement to receive cheques.
type Swap interface {
	// ReceiveCheque is called by the swap protocol if a cheque is received.
	ReceiveCheque(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque, exchange *big.Int, deduction *big.Int) error
	// Handshake is called by the swap protocol when a handshake is received.
	Handshake(peer swarm.Address, beneficiary common.Address) error
	GetDeductionForPeer(peer swarm.Address) (bool, error)
	GetDeductionByPeer(peer swarm.Address) (bool, error)
	AddDeductionByPeer(peer swarm.Address) error
}

// Service is the main implementation of the swap protocol.
type Service struct {
	streamer    p2p.Streamer
	logger      logging.Logger
	swap        Swap
	exchange    exchange.Service
	beneficiary common.Address
}

// New creates a new swap protocol Service.
func New(streamer p2p.Streamer, logger logging.Logger, beneficiary common.Address, exchange exchange.Service) *Service {
	return &Service{
		streamer:    streamer,
		logger:      logger,
		beneficiary: beneficiary,
		exchange:    exchange,
	}
}

// SetSwap sets the swap to notify.
func (s *Service) SetSwap(swap Swap) {
	s.swap = swap
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Handler: s.handler,
				Headler: s.headler,
			},
			{
				Name:    initStreamName,
				Handler: s.initHandler,
			},
		},
		ConnectOut: s.init,
	}
}

func (s *Service) initHandler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	var req pb.Handshake
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	if len(req.Beneficiary) != 20 {
		return errors.New("malformed beneficiary address")
	}

	err = w.WriteMsgWithContext(ctx, &pb.Handshake{
		Beneficiary: s.beneficiary.Bytes(),
	})
	if err != nil {
		return err
	}

	beneficiary := common.BytesToAddress(req.Beneficiary)
	return s.swap.Handshake(p.Address, beneficiary)
}

// init is called on outgoing connections and triggers handshake exchange
func (s *Service) init(ctx context.Context, p p2p.Peer) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stream, err := s.streamer.NewStream(ctx, p.Address, nil, protocolName, protocolVersion, initStreamName)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose() // wait for confirmation
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	err = w.WriteMsgWithContext(ctx, &pb.Handshake{
		Beneficiary: s.beneficiary.Bytes(),
	})
	if err != nil {
		return err
	}

	var req pb.Handshake
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	// any 20-byte byte-sequence is a valid eth address
	if len(req.Beneficiary) != 20 {
		return errors.New("malformed beneficiary address")
	}

	beneficiary := common.BytesToAddress(req.Beneficiary)

	return s.swap.Handshake(p.Address, beneficiary)
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

	var req pb.EmitCheque
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	responseHeaders := stream.ResponseHeaders()
	exchange, deduction, err := swap.ParseSettlementResponseHeaders(responseHeaders)
	if err != nil {
		if !errors.Is(err, swap.ErrNoDeductionHeader) {
			return err
		}
		deduction = big.NewInt(0)
	}

	var signedCheque *chequebook.SignedCheque
	err = json.Unmarshal(req.Cheque, &signedCheque)
	if err != nil {
		return err
	}

	// signature validation
	return s.swap.ReceiveCheque(ctx, p.Address, signedCheque, exchange, deduction)
}

func (s *Service) headler(receivedHeaders p2p.Headers, peerAddress swarm.Address) (returnHeaders p2p.Headers) {

	exchange, deduction, err := s.exchange.CurrentRates()
	if err != nil {
		return p2p.Headers{}
	}

	checkPeer, err := s.swap.GetDeductionForPeer(peerAddress)
	if err != nil {
		return p2p.Headers{}
	}

	if checkPeer {
		deduction = big.NewInt(0)
	}

	returnHeaders = swap.MakeSettlementHeaders(exchange, deduction)
	return
}

// InitiateCheque attempts to send a cheque to a peer.
func (s *Service) EmitCheque(ctx context.Context, peer swarm.Address, beneficiary common.Address, amount *big.Int, issue IssueFunc) (balance *big.Int, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	// reading exchanged headers
	returnedHeaders := stream.Headers()
	exchange, deduction, err := swap.ParseSettlementResponseHeaders(returnedHeaders)
	if err != nil {
		if !errors.Is(err, swap.ErrNoDeductionHeader) {
			return nil, err
		}
		deduction = big.NewInt(0)
	}

	// comparing received headers to known truth

	// get whether peer have deducted in the past
	checkPeer, err := s.swap.GetDeductionByPeer(peer)
	if err != nil {
		return nil, err
	}

	// if peer is not entitled for deduction but sent non zero deduction value, return with error
	if checkPeer && deduction.Cmp(big.NewInt(0)) != 0 {
		return nil, ErrHaveDeduction
	}

	// get current global exchange rate and deduction
	checkExchange, checkDeduction, err := s.exchange.CurrentRates()
	if err != nil {
		return nil, err
	}

	// exchange rates should match
	if exchange.Cmp(checkExchange) != 0 {
		return nil, ErrNegotiateRate
	}

	// deduction values should match or be zero
	if deduction.Cmp(checkDeduction) != 0 && deduction.Cmp(big.NewInt(0)) != 0 {
		return nil, ErrNegotiateDeduction
	}

	paymentAmount := new(big.Int).Mul(amount, exchange)
	sentAmount := new(big.Int).Add(paymentAmount, deduction)

	// issue cheque call with provided callback for sending cheque to finish transaction

	balance, err = issue(ctx, beneficiary, sentAmount, func(cheque *chequebook.SignedCheque) error {
		// for simplicity we use json marshaller. can be replaced by a binary encoding in the future.
		encodedCheque, err := json.Marshal(cheque)
		if err != nil {
			return err
		}

		// sending cheque
		s.logger.Tracef("sending cheque message to peer %v (%v)", peer, cheque)

		w := protobuf.NewWriter(stream)
		return w.WriteMsgWithContext(ctx, &pb.EmitCheque{
			Cheque: encodedCheque,
		})

	})
	if err != nil {
		return nil, err
	}

	err = s.swap.AddDeductionByPeer(peer)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

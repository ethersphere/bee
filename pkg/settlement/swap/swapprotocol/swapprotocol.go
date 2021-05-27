// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/headerutils"
	"github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName    = "swap"
	protocolVersion = "1.0.0"
	streamName      = "swap" // stream for cheques
	initStreamName  = "init" // stream for handshake
)

type SendChequeFunc func(cheque *SignedCheque) error

type IssueFunc func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc SendChequeFunc) (*big.Int, error)

// Interface is the main interface to send messages over swap protocol.
type Interface interface {
	// EmitCheque sends a signed cheque to a peer.
	EmitCheque(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque) error
}

// Swap is the interface the settlement layer should implement to receive cheques.
type Swap interface {
	// ReceiveCheque is called by the swap protocol if a cheque is received.
	ReceiveCheque(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque) error
	// Handshake is called by the swap protocol when a handshake is received.
	Handshake(peer swarm.Address, beneficiary common.Address) error
}

// Service is the main implementation of the swap protocol.
type Service struct {
	streamer    p2p.Streamer
	logger      logging.Logger
	swap        Swap
	beneficiary common.Address
}

// New creates a new swap protocol Service.
func New(streamer p2p.Streamer, logger logging.Logger, beneficiary common.Address) *Service {
	return &Service{
		streamer:    streamer,
		logger:      logger,
		beneficiary: beneficiary,
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

	var signedCheque *chequebook.SignedCheque
	err = json.Unmarshal(req.Cheque, &signedCheque)
	if err != nil {
		return err
	}

	return s.swap.ReceiveCheque(ctx, p.Address, signedCheque)
}

func (s *Service) headler(receivedHeaders p2p.Headers, peerAddress swarm.Address) (returnHeaders p2p.Headers) {

	exchange := uint64(347)
	deductionFromPeer := uint64(300)

	returnHeaders = swap.MakeSettlementHeaders(exchange, deductionFromPeer)

	return
}

// InitiateCheque attempts to send a cheque to a peer.
func (s *Service) InitiateCheque(ctx context.Context, peer swarm.Address, amount *big.Int, issue IssueFunc) error {
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
			_ = stream.FullClose()
		}
	}()

	// exchanging header

	// comparing received headers to known truth

	// issue cheque

	amount, err := issue(ctx, beneficiary, amount, func(cheque *chequebook.SignedCheque) error {
		// for simplicity we use json marshaller. can be replaced by a binary encoding in the future.
		encodedCheque, err := json.Marshal(cheque)
		if err != nil {
			return err
		}

		s.logger.Tracef("sending cheque message to peer %v (%v)", peer, cheque)

		w := protobuf.NewWriter(stream)
		return w.WriteMsgWithContext(ctx, &pb.EmitCheque{
			Cheque: encodedCheque,
		})
		// sending cheque
	})

}

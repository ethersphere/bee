// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package chainsync provides the implementation
// of the chainsync protocol that verifies peer chain syncronization.
package chainsync

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethersphere/bee/pkg/chainsync/pb"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/ratelimit"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
)

const (
	protocolName    = "chainsync"
	protocolVersion = "1.0.0"
	syncStreamName  = "prove"
)

var (
	messageTimeout       = time.Second * 30
	limitBurst           = 2
	limitRate            = time.Second * 30
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

func (s *cs) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    syncStreamName,
				Handler: s.syncHandler,
			},
		},
		DisconnectIn:  nil,
		DisconnectOut: nil,
	}
}

type cs struct {
	streamer  p2p.Streamer
	ethClient transaction.Backend

	inLimiter  *ratelimit.Limiter
	outLimiter *ratelimit.Limiter

	quit chan struct{}
}

func New(s p2p.Streamer, backend transaction.Backend) *cs {
	c := &cs{
		streamer:   s,
		ethClient:  backend,
		inLimiter:  ratelimit.New(limitRate, limitBurst),
		outLimiter: ratelimit.New(limitRate, limitBurst),

		quit: make(chan struct{}),
	}
	return c
}

// Prove asks a peer to prove they know a certain block height on the
// current used eth backend.
func (s *cs) Prove(ctx context.Context, peer swarm.Address, blockheight uint64) ([]byte, error) {
	if !s.outLimiter.Allow(peer.ByteString(), 1) {
		return nil, ErrRateLimitExceeded
	}
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, syncStreamName)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, messageTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)

	intBuffer := make([]byte, 8)
	n := binary.PutUvarint(intBuffer, blockheight)

	var desc = pb.Describe{BlockHeight: intBuffer[:n]}
	if err := w.WriteMsgWithContext(ctx, &desc); err != nil {
		return nil, fmt.Errorf("write describe message: %w", err)
	}

	var proof pb.Proof
	if err := r.ReadMsgWithContext(ctx, &proof); err != nil {
		return nil, fmt.Errorf("read proof message: %w", err)
	}
	return proof.BlockHash, nil
}

func (s *cs) syncHandler(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
	w, r := protobuf.NewWriterAndReader(stream)
	ctx, cancel := context.WithTimeout(ctx, messageTimeout)
	defer cancel()
	var describe pb.Describe
	if err := r.ReadMsgWithContext(ctx, &describe); err != nil {
		_ = stream.Reset()
		return fmt.Errorf("read describe: %w", err)
	}

	if !s.inLimiter.Allow(peer.Address.ByteString(), 1) {
		_ = stream.Reset()
		return ErrRateLimitExceeded
	}
	height, _ := binary.Uvarint(describe.BlockHeight)
	header, err := s.ethClient.HeaderByNumber(ctx, new(big.Int).SetUint64(height))
	if err != nil {
		stream.Reset()
		return fmt.Errorf("header by number: %w", err)
	}

	blockHash := header.Hash().Bytes()
	var proof = pb.Proof{BlockHash: blockHash}
	if err := w.WriteMsgWithContext(ctx, &proof); err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	return nil
}

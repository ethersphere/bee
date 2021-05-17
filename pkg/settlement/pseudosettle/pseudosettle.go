// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
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

var (
	SettlementReceivedPrefix = "pseudosettle_total_received_"
	SettlementSentPrefix     = "pseudosettle_total_sent_"
)

type Service struct {
	streamer      p2p.Streamer
	logger        logging.Logger
	store         storage.StateStorer
	accountingAPI settlement.AccountingAPI
	metrics       metrics
}

func New(streamer p2p.Streamer, logger logging.Logger, store storage.StateStorer, accountingAPI settlement.AccountingAPI) *Service {
	return &Service{
		streamer:      streamer,
		logger:        logger,
		metrics:       newMetrics(),
		store:         store,
		accountingAPI: accountingAPI,
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

func totalKey(peer swarm.Address, prefix string) string {
	return fmt.Sprintf("%v%v", prefix, peer.String())
}

func totalKeyPeer(key []byte, prefix string) (peer swarm.Address, err error) {
	k := string(key)

	split := strings.SplitAfter(k, prefix)
	if len(split) != 2 {
		return swarm.ZeroAddress, errors.New("no peer in key")
	}
	return swarm.ParseHexAddress(split[1])
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
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	s.metrics.TotalReceivedPseudoSettlements.Add(float64(req.Amount))
	s.logger.Tracef("received payment message from peer %v of %d", p.Address, req.Amount)

	totalReceived, err := s.TotalReceived(p.Address)
	if err != nil {
		if !errors.Is(err, settlement.ErrPeerNoSettlements) {
			return err
		}
		totalReceived = big.NewInt(0)
	}

	err = s.store.Put(totalKey(p.Address, SettlementReceivedPrefix), totalReceived.Add(totalReceived, new(big.Int).SetUint64(req.Amount)))
	if err != nil {
		return err
	}

	return s.accountingAPI.NotifyPaymentReceived(p.Address, new(big.Int).SetUint64(req.Amount))
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount *big.Int) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var err error
	defer func() {
		if err != nil {
			s.accountingAPI.NotifyPaymentSent(peer, nil, err)
		}
	}()
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return
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
		Amount: amount.Uint64(),
	})
	if err != nil {
		return
	}
	totalSent, err := s.TotalSent(peer)
	if err != nil {
		if !errors.Is(err, settlement.ErrPeerNoSettlements) {
			return
		}
		totalSent = big.NewInt(0)
	}

	err = s.store.Put(totalKey(peer, SettlementSentPrefix), totalSent.Add(totalSent, amount))
	if err != nil {
		return
	}
	s.accountingAPI.NotifyPaymentSent(peer, amount, nil)
	amountFloat, _ := new(big.Float).SetInt(amount).Float64()
	s.metrics.TotalSentPseudoSettlements.Add(amountFloat)
}

func (s *Service) SetAccountingAPI(accountingAPI settlement.AccountingAPI) {
	s.accountingAPI = accountingAPI
}

// TotalSent returns the total amount sent to a peer
func (s *Service) TotalSent(peer swarm.Address) (totalSent *big.Int, err error) {
	key := totalKey(peer, SettlementSentPrefix)
	err = s.store.Get(key, &totalSent)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, settlement.ErrPeerNoSettlements
		}
		return nil, err
	}
	return totalSent, nil
}

// TotalReceived returns the total amount received from a peer
func (s *Service) TotalReceived(peer swarm.Address) (totalReceived *big.Int, err error) {
	key := totalKey(peer, SettlementReceivedPrefix)
	err = s.store.Get(key, &totalReceived)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, settlement.ErrPeerNoSettlements
		}
		return nil, err
	}
	return totalReceived, nil
}

// SettlementsSent returns all stored sent settlement values for a given type of prefix
func (s *Service) SettlementsSent() (map[string]*big.Int, error) {
	sent := make(map[string]*big.Int)
	err := s.store.Iterate(SettlementSentPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := totalKeyPeer(key, SettlementSentPrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}
		if _, ok := sent[addr.String()]; !ok {
			var storevalue *big.Int
			err = s.store.Get(totalKey(addr, SettlementSentPrefix), &storevalue)
			if err != nil {
				return false, fmt.Errorf("get peer %s settlement balance: %w", addr.String(), err)
			}

			sent[addr.String()] = storevalue
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return sent, nil
}

// SettlementsReceived returns all stored received settlement values for a given type of prefix
func (s *Service) SettlementsReceived() (map[string]*big.Int, error) {
	received := make(map[string]*big.Int)
	err := s.store.Iterate(SettlementReceivedPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := totalKeyPeer(key, SettlementReceivedPrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}
		if _, ok := received[addr.String()]; !ok {
			var storevalue *big.Int
			err = s.store.Get(totalKey(addr, SettlementReceivedPrefix), &storevalue)
			if err != nil {
				return false, fmt.Errorf("get peer %s settlement balance: %w", addr.String(), err)
			}

			received[addr.String()] = storevalue
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return received, nil
}

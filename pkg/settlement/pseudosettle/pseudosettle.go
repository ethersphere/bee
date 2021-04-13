// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle/headerutils"
	pb "github.com/ethersphere/bee/pkg/settlement/pseudosettle/pb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	protocolName    = "pseudosettle"
	protocolVersion = "1.0.0"
	streamName      = "pseudosettle"
)

const (
	refreshRate = int64(10000000000000)
)

var (
	SettlementReceivedPrefix = "pseudosettle_total_received_"
	SettlementSentPrefix     = "pseudosettle_total_sent_"

	SettlementReceivedTimestampPrefix = "pseudosettle_timestamp_received_"
	SettlementSentTimestampPrefix     = "pseudosettle_timestamp_sent_"
)

type Service struct {
	streamer          p2p.Streamer
	logger            logging.Logger
	store             storage.StateStorer
	notifyPaymentFunc settlement.NotifyPaymentFunc
	metrics           metrics
}

func New(streamer p2p.Streamer, logger logging.Logger, store storage.StateStorer) *Service {
	return &Service{
		streamer: streamer,
		logger:   logger,
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
				Headler: s.headler,
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

func (s *Service) peerAllowance(peer swarm.Address) (limit big.Int, stamp int64) {
	currentTime := time.Now().Unix()
	return big.NewInt(10000), currentTime
}

func (s *Service) settlementAllowedHeadler(receivedHeaders p2p.Headers, peerAddress swarm.Address) (returnHeaders p2p.Headers) {

	allowedLimit, timestamp := s.peerAllowance(peerAddress)

	returnHeaders, err = headerutils.MakeSettlementResponseHeaders(allowedLimit, timestamp)
	if err != nil {
		return p2p.Headers{
			"error": []byte("Error creating response allowance headers"),
		}
	}
	s.logger.Debugf("settlement headler: response with allowance as %v, timestamp as %v, for peer %s", allowedLimit, timestamp, peerAddress)

	return returnHeaders
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	r := protobuf.NewReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()
	var req pb.Payment
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	s.metrics.TotalReceivedPseudoSettlements.Add(float64(req.Amount))
	s.logger.Tracef("pseudosettle received payment message from peer %v of %d", p.Address, req.Amount)

	totalReceived, err := s.TotalReceived(p.Address)
	if err != nil {
		if !errors.Is(err, settlement.ErrPeerNoSettlements) {
			return err
		}
		totalReceived = big.NewInt(0)
	}

	var lastTime int64
	err = s.store.Get(totalKey(p.Address, SettlementReceivedTimestampPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		lastTime = 0
	}

	if req.Timestamp <= uint64(lastTime) {
		return errors.New("pseudosettle time not increasing")
	}

	currentTime := time.Now().Unix()
	if math.Abs(float64(currentTime-int64(req.Timestamp))) > 5 {
		return errors.New("pseudosettle time difference is too big")
	}

	maxAllowance := (int64(req.Timestamp) - lastTime) * refreshRate
	if req.Amount > uint64(maxAllowance) {
		s.logger.Trace("pseudosettle allowance exceeded")
		return fmt.Errorf("pseudosettle allowance exceeded. amount was %d, should have been %d max", req.Amount, maxAllowance)
	}

	err = s.store.Put(totalKey(p.Address, SettlementReceivedPrefix), totalReceived.Add(totalReceived, new(big.Int).SetUint64(req.Amount)))
	if err != nil {
		return err
	}

	err = s.store.Put(totalKey(p.Address, SettlementReceivedTimestampPrefix), req.Timestamp)
	if err != nil {
		return err
	}

	return s.notifyPaymentFunc(p.Address, new(big.Int).SetUint64(req.Amount))
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount *big.Int) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var lastTime int64
	err := s.store.Get(totalKey(peer, SettlementSentTimestampPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		lastTime = 0
	}

	currentTime := time.Now().Unix()
	if currentTime == lastTime {
		return nil, errors.New("pseudosettle too soon")
	}

	maxAllowance := (currentTime - lastTime) * refreshRate

	if amount.Int64() > maxAllowance {
		s.logger.Infof("pseudosettle using reduced settlement %d instead of %d", maxAllowance, amount)
		amount = new(big.Int).SetInt64(maxAllowance)
	}

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	s.logger.Tracef("pseudosettle sending payment message to peer %v of %d", peer, amount)
	w := protobuf.NewWriter(stream)
	err = w.WriteMsgWithContext(ctx, &pb.Payment{
		Amount:    amount.Uint64(),
		Timestamp: uint64(currentTime),
	})
	if err != nil {
		return nil, err
	}
	totalSent, err := s.TotalSent(peer)
	if err != nil {
		if !errors.Is(err, settlement.ErrPeerNoSettlements) {
			return nil, err
		}
		totalSent = big.NewInt(0)
	}

	err = s.store.Put(totalKey(peer, SettlementSentPrefix), totalSent.Add(totalSent, amount))
	if err != nil {
		return nil, err
	}

	err = s.store.Put(totalKey(peer, SettlementSentTimestampPrefix), currentTime)
	if err != nil {
		return nil, err
	}

	amountFloat, _ := new(big.Float).SetInt(amount).Float64()
	s.metrics.TotalSentPseudoSettlements.Add(amountFloat)
	return amount, nil
}

// SetNotifyPaymentFunc sets the NotifyPaymentFunc to notify
func (s *Service) SetNotifyPaymentFunc(notifyPaymentFunc settlement.NotifyPaymentFunc) {
	s.notifyPaymentFunc = notifyPaymentFunc
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

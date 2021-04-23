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

	ErrSettlementTooSoon = errors.New("settlement too soon")
)

type Service struct {
	streamer      p2p.Streamer
	logger        logging.Logger
	store         storage.StateStorer
	accountingAPI settlement.AccountingAPI
	metrics       metrics
	refreshRate   *big.Int
	timeNow       func() time.Time
}

type lastPayment struct {
	Timestamp int64
	Total     *big.Int
}

func New(streamer p2p.Streamer, logger logging.Logger, store storage.StateStorer, accountingAPI settlement.AccountingAPI, refreshRate *big.Int) *Service {
	return &Service{
		streamer:      streamer,
		logger:        logger,
		metrics:       newMetrics(),
		store:         store,
		accountingAPI: accountingAPI,
		refreshRate:   refreshRate,
		timeNow:       time.Now,
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

// peerAllowance computes the maximum incoming payment value we accept
// this is the time based allowance or the peers actual debt, whichever is less
func (s *Service) peerAllowance(peer swarm.Address) (limit *big.Int, stamp int64, err error) {
	var lastTime lastPayment
	err = s.store.Get(totalKey(peer, SettlementReceivedPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, 0, err
		}
		lastTime.Timestamp = int64(0)
	}

	currentTime := s.timeNow().Unix()
	if currentTime == lastTime.Timestamp {
		return nil, 0, ErrSettlementTooSoon
	}

	maxAllowance := new(big.Int).Mul(big.NewInt(currentTime-lastTime.Timestamp), s.refreshRate)

	peerDebt, err := s.accountingAPI.PeerDebt(peer)
	if err != nil {
		return nil, 0, err
	}

	if peerDebt.Cmp(maxAllowance) >= 0 {
		return maxAllowance, currentTime, nil
	}

	return peerDebt, currentTime, nil
}

func (s *Service) headler(receivedHeaders p2p.Headers, peerAddress swarm.Address) (returnHeaders p2p.Headers) {
	allowedLimit, timestamp, err := s.peerAllowance(peerAddress)
	if err != nil {
		return p2p.Headers{}
	}

	returnHeaders, err = MakeAllowanceResponseHeaders(allowedLimit, timestamp)
	if err != nil {
		return p2p.Headers{}
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

	responseHeaders := stream.ResponseHeaders()

	allowance, timestamp, err := ParseAllowanceResponseHeaders(responseHeaders)
	if err != nil {
		return err
	}

	receivedPayment := big.NewInt(0).SetBytes(req.Amount)

	if allowance.Cmp(receivedPayment) < 0 {
		s.logger.Trace("pseudosettle allowance exceeded")
		return fmt.Errorf("pseudosettle allowance exceeded. amount was %d, should have been %d max", receivedPayment, allowance)
	}

	receivedPaymentF64, _ := big.NewFloat(0).SetInt(receivedPayment).Float64()

	s.metrics.TotalReceivedPseudoSettlements.Add(receivedPaymentF64)
	s.logger.Tracef("pseudosettle received payment message from peer %v of %d", p.Address, receivedPayment)

	var lastTime lastPayment
	err = s.store.Get(totalKey(p.Address, SettlementReceivedPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		lastTime.Total = big.NewInt(0)
	}

	lastTime.Total = lastTime.Total.Add(lastTime.Total, receivedPayment)
	lastTime.Timestamp = timestamp

	err = s.store.Put(totalKey(p.Address, SettlementReceivedPrefix), lastTime)
	if err != nil {
		return err
	}

	return s.accountingAPI.NotifyPaymentReceived(p.Address, receivedPayment)
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

	var lastTime lastPayment
	err = s.store.Get(totalKey(peer, SettlementSentPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return
		}
		lastTime.Total = big.NewInt(0)
		lastTime.Timestamp = 0
	}

	currentTime := s.timeNow().Unix()
	if currentTime == lastTime.Timestamp {
		err = ErrSettlementTooSoon
		return
	}

	// maxAllowance := new(big.Int).Mul(big.NewInt(currentTime-lastTime), refreshRate)

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	returnedHeaders := stream.Headers()

	allowance, timestamp, err := ParseAllowanceResponseHeaders(returnedHeaders)
	if err != nil {
		return
	}

	if amount.Cmp(allowance) > 0 {
		s.logger.Infof("pseudosettle using reduced settlement %d instead of %d", allowance, amount)
		amount = allowance
	}

	s.logger.Tracef("pseudosettle sending payment message to peer %v of %d", peer, amount)
	w := protobuf.NewWriter(stream)
	err = w.WriteMsgWithContext(ctx, &pb.Payment{
		Amount: amount.Bytes(),
	})
	if err != nil {
		return
	}

	lastTime.Total = lastTime.Total.Add(lastTime.Total, amount)
	lastTime.Timestamp = timestamp

	err = s.store.Put(totalKey(peer, SettlementSentPrefix), lastTime)
	if err != nil {
		return
	}

	s.accountingAPI.NotifyPaymentSent(peer, amount, nil)
	amountFloat, _ := new(big.Float).SetInt(amount).Float64()
	if amountFloat < 0 {
		fmt.Println(amount)
	}
	s.metrics.TotalSentPseudoSettlements.Add(amountFloat)
}

func (s *Service) SetAccountingAPI(accountingAPI settlement.AccountingAPI) {
	s.accountingAPI = accountingAPI
}

// TotalSent returns the total amount sent to a peer
func (s *Service) TotalSent(peer swarm.Address) (totalSent *big.Int, err error) {
	var lastTime lastPayment

	err = s.store.Get(totalKey(peer, SettlementSentPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, settlement.ErrPeerNoSettlements
		}
		lastTime.Total = big.NewInt(0)
	}

	return lastTime.Total, nil
}

// TotalReceived returns the total amount received from a peer
func (s *Service) TotalReceived(peer swarm.Address) (totalReceived *big.Int, err error) {
	var lastTime lastPayment

	err = s.store.Get(totalKey(peer, SettlementReceivedPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, settlement.ErrPeerNoSettlements
		}
		lastTime.Total = big.NewInt(0)
	}

	return lastTime.Total, nil
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
			var storevalue lastPayment
			err = s.store.Get(totalKey(addr, SettlementSentPrefix), &storevalue)
			if err != nil {
				return false, fmt.Errorf("get peer %s settlement balance: %w", addr.String(), err)
			}

			sent[addr.String()] = storevalue.Total
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
			var storevalue lastPayment
			err = s.store.Get(totalKey(addr, SettlementReceivedPrefix), &storevalue)
			if err != nil {
				return false, fmt.Errorf("get peer %s settlement balance: %w", addr.String(), err)
			}

			received[addr.String()] = storevalue.Total
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return received, nil
}

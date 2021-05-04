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
	"sync"
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

	ErrSettlementTooSoon  = errors.New("settlement too soon")
	ErrNoPseudoSettlePeer = errors.New("settlement peer not found")
)

type Service struct {
	streamer      p2p.Streamer
	logger        logging.Logger
	store         storage.StateStorer
	accountingAPI settlement.AccountingAPI
	metrics       metrics
	refreshRate   *big.Int
	timeNow       func() time.Time
	peersMu       sync.Mutex
	peers         map[string]*pseudoSettlePeer
}

type pseudoSettlePeer struct {
	lock sync.Mutex // lock to be held during receiving a payment from this peer
}

type lastPayment struct {
	Timestamp      int64
	CheckTimestamp int64
	Total          *big.Int
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
		peers:         make(map[string]*pseudoSettlePeer),
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
		ConnectIn:     s.init,
		ConnectOut:    s.init,
		DisconnectIn:  s.terminate,
		DisconnectOut: s.terminate,
	}
}

func (s *Service) init(ctx context.Context, p p2p.Peer) error {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	_, ok := s.peers[p.Address.String()]
	if !ok {
		peerData := &pseudoSettlePeer{}
		s.peers[p.Address.String()] = peerData
	}

	return nil
}

func (s *Service) terminate(p p2p.Peer) error {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	delete(s.peers, p.Address.String())
	return nil
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

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	// lock peer here
	w, r := protobuf.NewWriterAndReader(stream)
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

	attemptedAmount := big.NewInt(0).SetBytes(req.Amount)
	paymentAmount := attemptedAmount

	s.peersMu.Lock()
	pseudoSettlePeer, ok := s.peers[p.Address.String()]
	if !ok {
		s.peersMu.Unlock()
		return ErrNoPseudoSettlePeer
	}
	s.peersMu.Unlock()

	pseudoSettlePeer.lock.Lock()
	defer pseudoSettlePeer.lock.Unlock()

	allowance, timestamp, err := s.peerAllowance(p.Address)
	if err != nil {
		return err
	}

	if allowance.Cmp(attemptedAmount) < 0 {
		paymentAmount = allowance
		s.logger.Trace("pseudosettle accepting reduced payment from peer %v of %d", p.Address, paymentAmount)
	} else {
		s.logger.Tracef("pseudosettle accepting payment message from peer %v of %d", p.Address, paymentAmount)
	}

	err = w.WriteMsgWithContext(ctx, &pb.PaymentAck{
		Amount:    paymentAmount.Bytes(),
		Timestamp: timestamp,
	})
	if err != nil {
		return err
	}

	var lastTime lastPayment
	err = s.store.Get(totalKey(p.Address, SettlementReceivedPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		lastTime.Total = big.NewInt(0)
	}

	lastTime.Total = lastTime.Total.Add(lastTime.Total, paymentAmount)
	lastTime.Timestamp = timestamp

	err = s.store.Put(totalKey(p.Address, SettlementReceivedPrefix), lastTime)
	if err != nil {
		return err
	}

	receivedPaymentF64, _ := big.NewFloat(0).SetInt(paymentAmount).Float64()
	s.metrics.TotalReceivedPseudoSettlements.Add(receivedPaymentF64)
	return s.accountingAPI.NotifyPaymentReceived(p.Address, paymentAmount)
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

	/*
		if amount.Cmp(allowance) > 0 {
			s.logger.Infof("pseudosettle using reduced settlement %d instead of %d", allowance, amount)
			amount = allowance
		}
	*/

	s.logger.Tracef("pseudosettle sending payment message to peer %v of %d", peer, amount)
	w, r := protobuf.NewWriterAndReader(stream)

	// balance_before

	err = w.WriteMsgWithContext(ctx, &pb.Payment{
		Amount: amount.Bytes(),
	})
	if err != nil {
		return
	}

	// shadowReserve :=

	var paymentAck pb.PaymentAck
	err = r.ReadMsgWithContext(ctx, &paymentAck)
	if err != nil {
		return
	}

	// balance_after

	checkTime := s.timeNow().Unix()

	acceptedAmount := new(big.Int).SetBytes(paymentAck.Amount)
	if acceptedAmount.Cmp(amount) > 0 {
		err = fmt.Errorf("pseudosettle other peer %v accepted payment larger than expected", peer)
		return
	}

	experiencedInterval := checkTime - lastTime.CheckTimestamp
	allegedInterval := paymentAck.Timestamp - lastTime.Timestamp

	if allegedInterval < 0 {
		return
	}

	experienceDifferenceRecent := paymentAck.Timestamp - checkTime

	if experienceDifferenceRecent < -2 || experienceDifferenceRecent > 2 {
		return
	}

	experienceDifferenceInterval := experiencedInterval - allegedInterval
	if experienceDifferenceInterval < -3 || experienceDifferenceInterval > 3 {
		return
	}
	// enforce allowance
	// check if value is appropriate
	// enforcedAllowance := min(allegedInterval * refreshRate, outstandingDebt - shadowReserve)

	lastTime.Total = lastTime.Total.Add(lastTime.Total, acceptedAmount)
	lastTime.Timestamp = paymentAck.Timestamp
	lastTime.CheckTimestamp = checkTime

	err = s.store.Put(totalKey(peer, SettlementSentPrefix), lastTime)
	if err != nil {
		return
	}

	s.accountingAPI.NotifyPaymentSent(peer, acceptedAmount, nil)
	amountFloat, _ := new(big.Float).SetInt(acceptedAmount).Float64()
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

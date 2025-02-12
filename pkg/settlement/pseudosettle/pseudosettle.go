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

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	pb "github.com/ethersphere/bee/v2/pkg/settlement/pseudosettle/pb"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "pseudosettle"

const (
	protocolName    = "pseudosettle"
	protocolVersion = "1.0.0"
	streamName      = "pseudosettle"
)

var (
	SettlementReceivedPrefix = "pseudosettle_total_received_"
	SettlementSentPrefix     = "pseudosettle_total_sent_"

	ErrSettlementTooSoon              = errors.New("settlement too soon")
	ErrNoPseudoSettlePeer             = errors.New("settlement peer not found")
	ErrDisconnectAllowanceCheckFailed = errors.New("settlement allowance below enforced amount")
	ErrTimeOutOfSyncAlleged           = errors.New("settlement allowance timestamps from peer were decreasing")
	ErrTimeOutOfSyncRecent            = errors.New("settlement allowance timestamps from peer differed from our measurement by more than 2 seconds")
	ErrTimeOutOfSyncInterval          = errors.New("settlement allowance interval from peer differed from local interval by more than 3 seconds")
	ErrRefreshmentBelowExpected       = errors.New("refreshment below expected")
	ErrRefreshmentAboveExpected       = errors.New("refreshment above expected")
)

type Service struct {
	streamer         p2p.Streamer
	logger           log.Logger
	store            storage.StateStorer
	accounting       settlement.Accounting
	metrics          metrics
	refreshRate      *big.Int
	lightRefreshRate *big.Int
	p2pService       p2p.Service
	timeNow          func() time.Time
	peersMu          sync.Mutex
	peers            map[string]*pseudoSettlePeer
}

type pseudoSettlePeer struct {
	lock     sync.Mutex // lock to be held during receiving a payment from this peer
	fullNode bool
}

type lastPayment struct {
	Timestamp      int64
	CheckTimestamp int64
	Total          *big.Int
}

func New(streamer p2p.Streamer, logger log.Logger, store storage.StateStorer, accounting settlement.Accounting, refreshRate, lightRefreshRate *big.Int, p2pService p2p.Service) *Service {
	return &Service{
		streamer:         streamer,
		logger:           logger.WithName(loggerName).Register(),
		metrics:          newMetrics(),
		store:            store,
		accounting:       accounting,
		p2pService:       p2pService,
		refreshRate:      refreshRate,
		lightRefreshRate: lightRefreshRate,
		timeNow:          time.Now,
		peers:            make(map[string]*pseudoSettlePeer),
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
		peerData := &pseudoSettlePeer{fullNode: p.FullNode}
		s.peers[p.Address.String()] = peerData
	}

	go s.accounting.Connect(p.Address, p.FullNode)
	return nil
}

func (s *Service) terminate(p p2p.Peer) error {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()

	delete(s.peers, p.Address.String())

	go s.accounting.Disconnect(p.Address)
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
func (s *Service) peerAllowance(peer swarm.Address, fullNode bool) (limit *big.Int, stamp int64, err error) {
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

	var refreshRateUsed *big.Int

	if fullNode {
		refreshRateUsed = s.refreshRate
	} else {
		refreshRateUsed = s.lightRefreshRate
	}

	maxAllowance := new(big.Int).Mul(big.NewInt(currentTime-lastTime.Timestamp), refreshRateUsed)

	peerDebt, err := s.accounting.PeerDebt(peer)
	if err != nil {
		return nil, 0, err
	}

	if peerDebt.Cmp(maxAllowance) >= 0 {
		return maxAllowance, currentTime, nil
	}

	return peerDebt, currentTime, nil
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	loggerV1 := s.logger.V(1).Register()

	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
			s.metrics.ReceivedPseudoSettlementsErrors.Inc()
		} else {
			stream.FullClose()
		}
	}()
	var req pb.Payment
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request from peer %v: %w", p.Address, err)
	}

	attemptedAmount := big.NewInt(0).SetBytes(req.Amount)

	paymentAmount := new(big.Int).Set(attemptedAmount)

	s.peersMu.Lock()
	pseudoSettlePeer, ok := s.peers[p.Address.String()]
	s.peersMu.Unlock()
	if !ok {
		return ErrNoPseudoSettlePeer
	}

	pseudoSettlePeer.lock.Lock()
	defer pseudoSettlePeer.lock.Unlock()

	allowance, timestamp, err := s.peerAllowance(p.Address, pseudoSettlePeer.fullNode)
	if err != nil {
		return err
	}

	if allowance.Cmp(attemptedAmount) < 0 {
		paymentAmount.Set(allowance)
	}
	loggerV1.Debug("pseudosettle accepting payment message from peer", "peer_address", p.Address, "amount", paymentAmount)

	if paymentAmount.Cmp(big.NewInt(0)) < 0 {
		paymentAmount.Set(big.NewInt(0))
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
	s.metrics.ReceivedPseudoSettlements.Inc()
	return s.accounting.NotifyRefreshmentReceived(p.Address, paymentAmount, timestamp)
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount *big.Int) {
	loggerV1 := s.logger.V(1).Register()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var err error

	defer func() {
		if err != nil {
			s.metrics.SentPseudoSettlementsErrors.Inc()
		}
	}()

	var lastTime lastPayment
	err = s.store.Get(totalKey(peer, SettlementSentPrefix), &lastTime)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, err)
			return
		}
		lastTime.Total = big.NewInt(0)
		lastTime.Timestamp = 0
	}

	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, err)
		return
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	loggerV1.Debug("pseudosettle sending payment message to peer", "peer_address", peer, "amount", amount)
	w, r := protobuf.NewWriterAndReader(stream)

	err = w.WriteMsgWithContext(ctx, &pb.Payment{
		Amount: amount.Bytes(),
	})
	if err != nil {
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, err)
		return
	}

	var paymentAck pb.PaymentAck
	err = r.ReadMsgWithContext(ctx, &paymentAck)
	if err != nil {
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, err)
		return
	}

	checkTime := s.timeNow().UnixMilli()

	acceptedAmount := new(big.Int).SetBytes(paymentAck.Amount)
	if acceptedAmount.Cmp(amount) > 0 {
		err = fmt.Errorf("pseudosettle: peer %v: %w", peer, ErrRefreshmentAboveExpected)
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, err)
		return
	}

	experiencedInterval := checkTime/1000 - lastTime.CheckTimestamp
	allegedInterval := paymentAck.Timestamp - lastTime.Timestamp

	if allegedInterval < 0 {
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, ErrTimeOutOfSyncAlleged)
		return
	}

	experienceDifferenceRecent := paymentAck.Timestamp - checkTime/1000

	if experienceDifferenceRecent < -2 || experienceDifferenceRecent > 2 {
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, ErrTimeOutOfSyncRecent)
		return
	}

	experienceDifferenceInterval := experiencedInterval - allegedInterval
	if experienceDifferenceInterval < -3 || experienceDifferenceInterval > 3 {
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, ErrTimeOutOfSyncInterval)
		return
	}

	lastTime.Total = lastTime.Total.Add(lastTime.Total, acceptedAmount)
	lastTime.Timestamp = paymentAck.Timestamp
	lastTime.CheckTimestamp = checkTime / 1000

	err = s.store.Put(totalKey(peer, SettlementSentPrefix), lastTime)
	if err != nil {
		s.accounting.NotifyRefreshmentSent(peer, nil, nil, 0, 0, err)
		return
	}

	amountFloat, _ := new(big.Float).SetInt(acceptedAmount).Float64()
	s.metrics.TotalSentPseudoSettlements.Add(amountFloat)
	s.metrics.SentPseudoSettlements.Inc()

	s.accounting.NotifyRefreshmentSent(peer, amount, acceptedAmount, checkTime, allegedInterval, nil)
}

func (s *Service) SetAccounting(accounting settlement.Accounting) {
	s.accounting = accounting
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

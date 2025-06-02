//go:build js
// +build js

package pseudosettle

import (
	"context"
	"errors"
	"fmt"
	"math/big"
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

type Service struct {
	streamer         p2p.Streamer
	logger           log.Logger
	store            storage.StateStorer
	accounting       settlement.Accounting
	refreshRate      *big.Int
	lightRefreshRate *big.Int
	p2pService       p2p.Service
	timeNow          func() time.Time
	peersMu          sync.Mutex
	peers            map[string]*pseudoSettlePeer
}

func New(streamer p2p.Streamer, logger log.Logger, store storage.StateStorer, accounting settlement.Accounting, refreshRate, lightRefreshRate *big.Int, p2pService p2p.Service) *Service {
	return &Service{
		streamer:         streamer,
		logger:           logger.WithName(loggerName).Register(),
		store:            store,
		accounting:       accounting,
		p2pService:       p2pService,
		refreshRate:      refreshRate,
		lightRefreshRate: lightRefreshRate,
		timeNow:          time.Now,
		peers:            make(map[string]*pseudoSettlePeer),
	}
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	loggerV1 := s.logger.V(1).Register()

	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			_ = stream.Reset()
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

	return s.accounting.NotifyRefreshmentReceived(p.Address, paymentAmount, timestamp)
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount *big.Int) {
	loggerV1 := s.logger.V(1).Register()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var err error

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

	s.accounting.NotifyRefreshmentSent(peer, amount, acceptedAmount, checkTime, allegedInterval, nil)
}

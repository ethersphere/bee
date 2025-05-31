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

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/settlement"
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

type pseudoSettlePeer struct {
	lock     sync.Mutex // lock to be held during receiving a payment from this peer
	fullNode bool
}

type lastPayment struct {
	Timestamp      int64
	CheckTimestamp int64
	Total          *big.Int
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

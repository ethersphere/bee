// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsyncer_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/chainsyncer"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
)

// nolint:paralleltest
func TestChainsyncer(t *testing.T) {
	var (
		expBlockHash    = common.HexToHash("0x9de2787d1d80a6164f4bc6359d9017131cbc14402ee0704bff0c6d691701c1dc").Bytes()
		logger          = log.Noop
		trxBlock        = common.HexToHash("0x2")
		blockC          = make(chan struct{})
		nextBlockHeader = &types.Header{ParentHash: trxBlock}
		blockNumber     = backendmock.WithBlockNumberFunc(func(_ context.Context) (uint64, error) { return uint64(100), nil })
		headerByNum     = backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			return nextBlockHeader, nil
		})

		backend        = backendmock.New(headerByNum, blockNumber)
		topology       = mock.NewTopologyDriver(mock.WithPeers(swarm.NewAddress([]byte{0, 1, 2, 3})))
		proofBlockHash = make([]byte, 32)
		proofError     = errors.New("error")
		p              = &prover{f: func(_ swarm.Address, _ uint64) ([]byte, error) {
			return proofBlockHash, proofError
		}}
		d = &m{f: func(_ swarm.Address, _ time.Duration) {
			select {
			case blockC <- struct{}{}:
			default:
			}
		}}
	)

	newChainSyncerTest := func(e error, blockHash []byte, cb func(*testing.T)) func(*testing.T) {
		proofError = e
		proofBlockHash = blockHash
		return func(t *testing.T) {
			t.Helper()

			cs, err := chainsyncer.New(backend, p, topology, d, logger, &chainsyncer.Options{
				FlagTimeout:     1500 * time.Millisecond,
				PollEvery:       1100 * time.Millisecond,
				BlockerPollTime: 1100 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}

			defer cs.Close()
			cb(t)
		}
	}

	t.Run("prover error", newChainSyncerTest(proofError, proofBlockHash, func(t *testing.T) {
		t.Helper()

		select {
		case <-blockC:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for blocklisting")
		}
	}))

	t.Run("blockhash mismatch", newChainSyncerTest(nil, proofBlockHash, func(t *testing.T) {
		t.Helper()

		select {
		case <-blockC:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for blocklisting")
		}
	}))

	t.Run("all good", newChainSyncerTest(nil, expBlockHash, func(t *testing.T) {
		t.Helper()

		select {
		case <-blockC:
			t.Fatal("blocklisting occurred but should not have")
		case <-time.After(500 * time.Millisecond):
		}
	}))
}

type prover struct {
	f func(swarm.Address, uint64) ([]byte, error)
}

func (p *prover) Prove(_ context.Context, a swarm.Address, b uint64) ([]byte, error) {
	return p.f(a, b)
}

type m struct {
	f func(swarm.Address, time.Duration)
}

func (m *m) Disconnect(_ swarm.Address, _ string) error {
	panic("not implemented")
}
func (m *m) Blocklist(overlay swarm.Address, duration time.Duration, reason string) error {
	m.f(overlay, duration)
	return nil
}

// NetworkStatus implements p2p.NetworkStatuser interface.
// It always returns p2p.NetworkStatusAvailable.
func (m *m) NetworkStatus() p2p.NetworkStatus {
	return p2p.NetworkStatusAvailable
}

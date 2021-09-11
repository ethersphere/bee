// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsyncer_test

import (
	"context"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/chainsyncer"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
)

func TestChainsyncer(t *testing.T) {
	var (
		expBlockHash    = common.HexToHash("0x9de2787d1d80a6164f4bc6359d9017131cbc14402ee0704bff0c6d691701c1dc").Bytes()
		logger          = logging.New(os.Stdout, 0)
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
		d2 = func() {
			select {
			case blockC <- struct{}{}:
			default:
			}
		}
	)

	newChainSyncerTest := func(e error, blockHash []byte, cb func()) func(*testing.T) {
		proofError = e
		proofBlockHash = blockHash
		return func(t *testing.T) {
			cleanup := chainsyncer.SetNotifyHook(d2)
			defer cleanup()
			cs, err := chainsyncer.New(backend, p, topology, logger, &chainsyncer.Options{
				FlagTimeout: 100 * time.Millisecond,
				PollEvery:   50 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}

			defer cs.Close()
			cb()
		}
	}

	t.Run("prover error", newChainSyncerTest(proofError, proofBlockHash, func() {
		select {
		case <-blockC:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for blocklisting")
		}
	}))

	t.Run("blockhash mismatch", newChainSyncerTest(nil, proofBlockHash, func() {
		select {
		case <-blockC:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for blocklisting")
		}
	}))

	t.Run("all good", newChainSyncerTest(nil, expBlockHash, func() {
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

// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package neighborhood

import (
	"context"
	"errors"
	"math/big"
	"math/rand"
	"sort"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
)

type RevealedEvent struct {
	RoundNumber       *big.Int
	Overlay           common.Hash
	Stake             *big.Int
	StakeDensity      *big.Int
	ReserveCommitment common.Hash
	Depth             uint8
}

const (
	DefaultBlocksPerPage uint64 = 5000
	DefaultPastRounds    uint64 = 1200
)

var ErrEmptyRounds = errors.New("no reveals were found")

type Backend interface {
	BlockNumber(ctx context.Context) (uint64, error)
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
}

// OptimalNeighborhood looks at the last n rounds of the redistribution game and picks a random neighborhood
// from the bottom 40% of least populated neighborhoods.
func OptimalNeighborhood(ctx context.Context, backend Backend, incentivesContractAddress common.Address, incentivesContractABI abi.ABI, blocksPerRound, rounds, page uint64, logger log.Logger) (swarm.Address, uint8, error) {

	rollback := blocksPerRound * rounds

	height, err := backend.BlockNumber(ctx)
	if err != nil {
		return swarm.ZeroAddress, 0, err
	}

	var startHeight uint64
	if height > rollback {
		startHeight = height - rollback
	}

	logger.Info("finding the optimal neighborhood to join", "start_height", startHeight, "current_height", height)

	reveals := map[uint8]map[string]int{}
	var depths [swarm.MaxBins]int

	for startHeight <= height {

		start := big.NewInt(int64(startHeight))
		to := big.NewInt(0).Add(start, big.NewInt(int64(page)))

		filter := ethereum.FilterQuery{
			FromBlock: start,
			ToBlock:   to,
			Addresses: []common.Address{incentivesContractAddress},
			Topics:    [][]common.Hash{{incentivesContractABI.Events["Revealed"].ID}},
		}

		events, err := backend.FilterLogs(ctx, filter)
		if err != nil {
			return swarm.ZeroAddress, 0, err
		}

		for _, e := range events {
			var revealEvent RevealedEvent
			err := transaction.ParseEvent(&incentivesContractABI, "Revealed", &revealEvent, e)
			if err != nil {
				return swarm.ZeroAddress, 0, err
			}

			overlay := firstNBits(revealEvent.Overlay.Bytes(), int(revealEvent.Depth))

			if reveals[revealEvent.Depth] == nil {
				reveals[revealEvent.Depth] = map[string]int{}
			}

			reveals[revealEvent.Depth][overlay.ByteString()]++
			depths[revealEvent.Depth]++
		}

		startHeight = to.Uint64() + 1
	}

	if len(reveals) == 0 {
		return swarm.ZeroAddress, 0, ErrEmptyRounds
	}

	mostCommonDepth := 0
	mostCommonDepthCount := 0
	for i, c := range depths {
		if c > mostCommonDepthCount {
			mostCommonDepthCount = c
			mostCommonDepth = i
		}
	}

	revealsAtDepth := reveals[uint8(mostCommonDepth)]

	keys := make([]string, 0, len(revealsAtDepth))
	for key := range revealsAtDepth {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		return revealsAtDepth[keys[i]] < revealsAtDepth[keys[j]]
	})

	index := int(float64(len(revealsAtDepth)) * 0.4)
	neigh := swarm.NewAddress([]byte(keys[rand.Intn(index)]))

	logger.Info("found an optimal neighborhood", "address", neigh)

	return neigh, uint8(mostCommonDepth), nil
}

func firstNBits(b []byte, bits int) swarm.Address {

	bytes := bits / 8
	leftover := bits % 8
	if leftover > 0 {
		bytes++
	}

	ret := make([]byte, bytes)
	copy(ret, b)

	if leftover > 0 {
		ret[bytes-1] >>= (8 - leftover)
		ret[bytes-1] <<= (8 - leftover)
	}

	return swarm.BytesToAddress(ret)
}

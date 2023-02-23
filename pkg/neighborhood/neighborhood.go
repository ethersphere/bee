// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package neighborhood

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"math/big"

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

var ErrEmptyRounds = errors.New("no reveals were found")

type Backend interface {
	BlockNumber(ctx context.Context) (uint64, error)
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
}

// look at the last n rounds of the redistribution game and find the smallest neighborhood using reveal counts.
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
	neighborhoods := neighborhoodPrefixes(mostCommonDepth)

	var smallestNeighborhood swarm.Address
	smallestNeighborhoodCount := math.MaxInt

	for _, n := range neighborhoods {
		c := revealsAtDepth[n.ByteString()]
		if c < smallestNeighborhoodCount {
			smallestNeighborhoodCount = c
			smallestNeighborhood = n
		}
	}

	logger.Info("found the most optimal neighborhood", "address", smallestNeighborhood, "reveals", smallestNeighborhoodCount)

	return smallestNeighborhood, uint8(mostCommonDepth), nil
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

	return bytesToAddr(ret)
}

func bytesToAddr(b []byte) swarm.Address {
	addr := make([]byte, swarm.HashSize)
	copy(addr, b)
	return swarm.NewAddress(addr)
}

func neighborhoodPrefixes(bits int) []swarm.Address {

	max := 1 << bits
	leftover := bits % 8

	ret := make([]swarm.Address, 0, max)

	for i := 0; i < max; i++ {
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(i))

		var addr []byte

		if bits <= 8 {
			addr = []byte{buf[0]}
		} else if bits <= 16 {
			addr = []byte{buf[0], buf[1]}
		} else if bits <= 24 {
			addr = []byte{buf[0], buf[1], buf[2]}
		} else if bits <= 32 {
			addr = []byte{buf[0], buf[1], buf[2], buf[3]}
		}

		if leftover > 0 {
			addr[len(addr)-1] <<= (8 - leftover)
		}

		ret = append(ret, bytesToAddr((addr)))
	}

	return ret
}

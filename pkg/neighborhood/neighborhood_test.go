// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package neighborhood_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	chaincfg "github.com/ethersphere/bee/pkg/config"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/neighborhood"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/util/abiutil"
)

var (
	redistributionContractAddress = common.HexToAddress("eeee")
	redistributionABI             = abiutil.MustParseABI(chaincfg.Testnet.RedistributionABI)
)

func TestOptimalNeighborhood(t *testing.T) {
	t.Parallel()

	neighborhoods := [8]swarm.Address{
		swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000"),
		swarm.MustParseHexAddress("2000000000000000000000000000000000000000000000000000000000000000"),
		swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"),
		swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000"),
		swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"),
		swarm.MustParseHexAddress("a000000000000000000000000000000000000000000000000000000000000000"),
		swarm.MustParseHexAddress("c000000000000000000000000000000000000000000000000000000000000000"),
		swarm.MustParseHexAddress("e000000000000000000000000000000000000000000000000000000000000000"),
	}

	mostCommonDepth := 3

	var addrs [8][]swarm.Address
	for i := 0; i < 8; i++ {
		for j := 0; j < i+1; j++ {
			addrs[i] = append(addrs[i], swarm.RandAddressAt(t, neighborhoods[i], mostCommonDepth))
		}
	}

	// startAt block 20 + 80 blocks of rounds = height 100
	lastNRounds := 8
	blocksPerRound := 10
	startBlock := 20

	createRound := func(round, count, depth int) []types.Log {
		ret := []types.Log{}
		for i := uint64(0); i < uint64(count); i++ {
			ret = append(ret, createEvent(uint64(startBlock+round*blocksPerRound)+i, uint64(round), addrs[round][i], uint8(depth)))
		}
		return ret
	}

	var logs []types.Log
	logs = append(logs, createRound(0, 1, mostCommonDepth)...)
	logs = append(logs, createRound(1, 2, mostCommonDepth)...)
	logs = append(logs, createRound(2, 3, mostCommonDepth)...)
	logs = append(logs, createRound(3, 4, mostCommonDepth)...)
	logs = append(logs, createRound(4, 5, mostCommonDepth)...)
	logs = append(logs, createRound(5, 6, mostCommonDepth)...)
	logs = append(logs, createRound(6, 7, mostCommonDepth)...)
	logs = append(logs, createRound(6, 1, mostCommonDepth-1)...) // create a round where depth 2 was used
	logs = append(logs, createRound(7, 8, mostCommonDepth)...)

	b := &backend{height: 100, logs: logs}

	addr, depth, err := neighborhood.OptimalNeighborhood(context.Background(), b, redistributionContractAddress,
		redistributionABI, uint64(blocksPerRound), uint64(lastNRounds), 10, log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	if depth != uint8(mostCommonDepth) {
		t.Fatalf("mismatched depth got %d want %d", depth, mostCommonDepth)
	}

	if !swarm.ContainsAddress(neighborhoods[:3], addr) { // index 3 is bottom 40%.
		t.Fatalf("wrong random least populated neighborhood got %s want %s", addr, neighborhoods[:3])
	}
}

type backend struct {
	height uint64
	logs   []types.Log
}

// BlockNumber(ctx context.Context) (uint64, error)
// FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)

func (b *backend) BlockNumber(ctx context.Context) (uint64, error) {
	return b.height, nil
}

func (b *backend) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {

	start := query.FromBlock.Uint64()
	to := query.ToBlock.Uint64()

	var ret []types.Log

	for _, l := range b.logs {
		if l.BlockNumber >= start && l.BlockNumber <= to {
			ret = append(ret, l)
		}
	}

	return ret, nil
}

func createEvent(block uint64, round uint64, overlay swarm.Address, depth uint8) types.Log {
	event := redistributionABI.Events["Revealed"]
	// RoundNumber       *big.Int
	// Overlay           common.Hash
	// Stake             *big.Int
	// StakeDensity      *big.Int
	// ReserveCommitment common.Hash
	// Depth             uint8

	b, err := event.Inputs.NonIndexed().Pack(
		big.NewInt(int64(round)),
		common.BytesToHash(overlay.Bytes()),
		big.NewInt(0),
		big.NewInt(0),
		common.HexToHash("0x0"),
		depth,
	)
	if err != nil {
		panic(err)
	}

	return types.Log{
		Data:        b,
		BlockNumber: block,
		Topics:      []common.Hash{event.ID}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

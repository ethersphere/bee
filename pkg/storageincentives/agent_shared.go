// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const loggerName = "storageincentives"

const (
	DefaultBlocksPerRound = 152
	DefaultBlocksPerPhase = DefaultBlocksPerRound / 4

	// min # of transactions our wallet should be able to cover
	minTxCountToCover = 15

	// average tx gas used by transactions issued from agent
	avgTxGas = 250_000
)

type ChainBackend interface {
	BlockNumber(context.Context) (uint64, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
}

type Health interface {
	IsHealthy() bool
}

func (a *Agent) handleCommit(ctx context.Context, round uint64) error {
	// commit event handler has to be guarded with lock to avoid
	// race conditions when handler is triggered again from sample phase
	a.commitLock.Lock()
	defer a.commitLock.Unlock()

	if _, exists := a.state.CommitKey(round); exists {
		// already committed on this round, phase is skipped
		return nil
	}

	// the sample has to come from previous round to be able to commit it
	sample, exists := a.state.SampleData(round - 1)
	if !exists {
		// In absence of sample, phase is skipped
		return nil
	}

	err := a.commit(ctx, sample, round)
	if err != nil {
		return err
	}

	a.state.SetLastPlayedRound(round)

	return nil
}

func (a *Agent) makeSample(ctx context.Context, committedDepth uint8) (SampleData, error) {
	salt, err := a.contract.ReserveSalt(ctx)
	if err != nil {
		return SampleData{}, err
	}

	timeLimiter, err := a.getPreviousRoundTime(ctx)
	if err != nil {
		return SampleData{}, err
	}

	rSample, err := a.store.ReserveSample(ctx, salt, committedDepth, uint64(timeLimiter), a.minBatchBalance())
	if err != nil {
		return SampleData{}, err
	}

	sampleHash, err := sampleHash(rSample.Items)
	if err != nil {
		return SampleData{}, err
	}

	sample := SampleData{
		Anchor1:            salt,
		ReserveSampleItems: rSample.Items,
		ReserveSampleHash:  sampleHash,
		StorageRadius:      committedDepth,
	}

	return sample, nil
}

func (a *Agent) minBatchBalance() *big.Int {
	cs := a.chainStateGetter.GetChainState()
	nextRoundBlockNumber := ((a.state.currentBlock() / a.blocksPerRound) + 2) * a.blocksPerRound
	difference := nextRoundBlockNumber - cs.Block
	minBalance := new(big.Int).Add(cs.TotalAmount, new(big.Int).Mul(cs.CurrentPrice, big.NewInt(int64(difference))))

	return minBalance
}

func (a *Agent) Close() error {
	close(a.quit)

	stopped := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("stopping incentives with ongoing worker goroutine")
	}
}

func (a *Agent) wrapCommit(storageRadius uint8, sample []byte, key []byte) ([]byte, error) {
	storageRadiusByte := []byte{storageRadius}

	data := append(a.overlay.Bytes(), storageRadiusByte...)
	data = append(data, sample...)
	data = append(data, key...)

	return crypto.LegacyKeccak256(data)
}

// Status returns the node status
func (a *Agent) Status() (*Status, error) {
	return a.state.Status()
}

type SampleWithProofs struct {
	Hash     swarm.Address                       `json:"hash"`
	Proofs   redistribution.ChunkInclusionProofs `json:"proofs"`
	Duration time.Duration                       `json:"duration"`
}

// SampleWithProofs is called only by rchash API
func (a *Agent) SampleWithProofs(
	ctx context.Context,
	anchor1 []byte,
	anchor2 []byte,
	storageRadius uint8,
) (SampleWithProofs, error) {
	sampleStartTime := time.Now()

	timeLimiter, err := a.getPreviousRoundTime(ctx)
	if err != nil {
		return SampleWithProofs{}, err
	}

	rSample, err := a.store.ReserveSample(ctx, anchor1, storageRadius, uint64(timeLimiter), a.minBatchBalance())
	if err != nil {
		return SampleWithProofs{}, err
	}

	hash, err := sampleHash(rSample.Items)
	if err != nil {
		return SampleWithProofs{}, fmt.Errorf("sample hash: %w", err)
	}

	proofs, err := makeInclusionProofs(rSample.Items, anchor1, anchor2)
	if err != nil {
		return SampleWithProofs{}, fmt.Errorf("make proofs: %w", err)
	}

	return SampleWithProofs{
		Hash:     hash,
		Proofs:   proofs,
		Duration: time.Since(sampleStartTime),
	}, nil
}

func (a *Agent) HasEnoughFundsToPlay(ctx context.Context) (*big.Int, bool, error) {
	balance, err := a.backend.BalanceAt(ctx, a.state.ethAddress, nil)
	if err != nil {
		return nil, false, err
	}

	price, err := a.backend.SuggestGasPrice(ctx)
	if err != nil {
		return nil, false, err
	}

	avgTxFee := new(big.Int).Mul(big.NewInt(avgTxGas), price)
	minBalance := new(big.Int).Mul(avgTxFee, big.NewInt(minTxCountToCover))

	return minBalance, balance.Cmp(minBalance) >= 1, nil
}

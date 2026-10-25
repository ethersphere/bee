// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/wrapped"
	"github.com/google/go-cmp/cmp"
)

func TestSuggestedFeeAndTip(t *testing.T) {
	t.Parallel()

	var (
		ctx              = context.Background()
		minimumGasTipCap = uint64(10)
		baseFee          = big.NewInt(100)
	)

	testCases := []struct {
		name              string
		gasPrice          *big.Int
		boostPercent      int
		mockSuggestGasTip *big.Int
		mockSuggestGasErr error
		mockHeader        *types.Header
		mockHeaderErr     error
		wantGasFeeCap     *big.Int
		wantGasTipCap     *big.Int
		wantErr           error
	}{
		{
			name:          "with gas price",
			gasPrice:      big.NewInt(1000),
			wantGasFeeCap: big.NewInt(1000),
			wantGasTipCap: big.NewInt(1000),
		},
		{
			name:          "with gas price and base fee",
			gasPrice:      big.NewInt(1000),
			mockHeader:    &types.Header{BaseFee: baseFee},
			wantGasFeeCap: big.NewInt(1000),
			wantGasTipCap: big.NewInt(900),
		},
		{
			name:              "suggest tip error",
			mockSuggestGasErr: errors.New("suggest tip error"),
			wantErr:           errors.New("failed to suggest gas tip cap: suggest tip error"),
		},
		{
			name:              "header error",
			mockSuggestGasTip: big.NewInt(20),
			mockHeaderErr:     errors.New("header error"),
			wantErr:           errors.New("failed to get latest block header: header error"),
		},
		{
			name:              "no base fee",
			mockSuggestGasTip: big.NewInt(20),
			mockHeader:        &types.Header{},
			wantErr:           wrapped.ErrEIP1559NotSupported,
		},
		{
			name:              "suggested tip > minimum",
			mockSuggestGasTip: big.NewInt(20),
			mockHeader:        &types.Header{BaseFee: baseFee},
			wantGasFeeCap:     big.NewInt(220), // 2*100 + 20
			wantGasTipCap:     big.NewInt(20),
		},
		{
			name:              "suggested tip < minimum",
			mockSuggestGasTip: big.NewInt(5),
			mockHeader:        &types.Header{BaseFee: baseFee},
			wantGasFeeCap:     big.NewInt(210), // 2*100 + 10
			wantGasTipCap:     big.NewInt(10),
		},
		{
			name:              "with boost",
			boostPercent:      10,
			mockSuggestGasTip: big.NewInt(20),
			mockHeader:        &types.Header{BaseFee: baseFee},
			wantGasFeeCap:     big.NewInt(222), // 2*100 + 22
			wantGasTipCap:     big.NewInt(22),  // 20 * 1.1
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := wrapped.NewBackend(
				backendmock.New(
					backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
						return tc.mockSuggestGasTip, tc.mockSuggestGasErr
					}),
					backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
						return tc.mockHeader, tc.mockHeaderErr
					}),
				),
				minimumGasTipCap,
				5*time.Second,
				90,
				wrapped.DefaultFeeHistoryBlockCount,
			)

			gasFeeCap, gasTipCap, err := backend.SuggestedFeeAndTip(ctx, tc.gasPrice, tc.boostPercent)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if err.Error() != tc.wantErr.Error() {
					t.Fatalf("unexpected error. want %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.wantGasFeeCap.String(), gasFeeCap.String()); diff != "" {
				t.Errorf("gasFeeCap mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tc.wantGasTipCap.String(), gasTipCap.String()); diff != "" {
				t.Errorf("gasTipCap mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// History tips below the node minimum are raised to it. Tips already above it stay put.
func TestSuggestedFeeAndTipsFromHistory_MinimumTip(t *testing.T) {
	t.Parallel()

	// Two reward rows make the medians low=15, market=55, aggressive=95.
	history := &ethereum.FeeHistory{
		BaseFee: []*big.Int{big.NewInt(1), big.NewInt(1000)},
		Reward: [][]*big.Int{
			{big.NewInt(10), big.NewInt(50), big.NewInt(90)},
			{big.NewInt(20), big.NewInt(60), big.NewInt(100)},
		},
	}

	tests := []struct {
		name                    string
		minimum                 uint64
		low, market, aggressive string
	}{
		{name: "no minimum leaves history tips", minimum: 0, low: "15", market: "55", aggressive: "95"},
		{name: "minimum raises only the tip below it", minimum: 40, low: "40", market: "55", aggressive: "95"},
		{name: "minimum above every tier raises all of them", minimum: 200, low: "200", market: "200", aggressive: "200"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := wrapped.NewBackend(
				backendmock.New(
					backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
						return history, nil
					}),
				),
				tc.minimum,
				5*time.Second,
				90,
				wrapped.DefaultFeeHistoryBlockCount,
			)

			got, err := backend.SuggestedFeeAndTipsFromHistory(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.LowTip.String() != tc.low || got.MarketTip.String() != tc.market || got.AggressiveTip.String() != tc.aggressive {
				t.Fatalf("got low %s market %s aggressive %s, want %s %s %s",
					got.LowTip, got.MarketTip, got.AggressiveTip, tc.low, tc.market, tc.aggressive)
			}
		})
	}
}

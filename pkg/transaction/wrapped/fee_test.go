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
	"github.com/ethersphere/bee/v2/pkg/transaction"
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

// Medians of the two reward rows are low=15, market=55, aggressive=95.
// The last base fee is the next block, so the fee cap is 2*1000 plus the tip.
// A minimum above a history tip raises the tip and the cap; a minimum below it leaves the tip.
func TestSuggestFeeHistory(t *testing.T) {
	t.Parallel()

	history := &ethereum.FeeHistory{
		BaseFee: []*big.Int{big.NewInt(1), big.NewInt(1000)},
		Reward: [][]*big.Int{
			{big.NewInt(10), big.NewInt(50), big.NewInt(90)},
			{big.NewInt(20), big.NewInt(60), big.NewInt(100)},
		},
	}

	for _, tc := range []struct {
		name    string
		minimum uint64
		tier    int
		tip     string
		feeCap  string
	}{
		{name: "no minimum/low", tier: transaction.FeePriorityLow, tip: "15", feeCap: "2015"},
		{name: "no minimum/market", tier: transaction.FeePriorityMarket, tip: "55", feeCap: "2055"},
		{name: "no minimum/aggressive", tier: transaction.FeePriorityAggressive, tip: "95", feeCap: "2095"},
		{name: "minimum above low", minimum: 40, tier: transaction.FeePriorityLow, tip: "40", feeCap: "2040"},
		{name: "minimum below market", minimum: 40, tier: transaction.FeePriorityMarket, tip: "55", feeCap: "2055"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := wrapped.NewBackend(
				backendmock.New(
					backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
						return history, nil
					}),
					backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
						t.Fatal("fee history already carries the next base fee")
						return nil, nil
					}),
				),
				tc.minimum,
				5*time.Second,
				90,
				wrapped.DefaultFeeHistoryBlockCount,
			)

			feeCap, tip, err := backend.SuggestFeeHistory(context.Background(), tc.tier)
			if err != nil {
				t.Fatal(err)
			}
			if tip.String() != tc.tip || feeCap.String() != tc.feeCap {
				t.Fatalf("got tip %s fee cap %s, want %s and %s", tip, feeCap, tc.tip, tc.feeCap)
			}
		})
	}

	t.Run("invalid tier", func(t *testing.T) {
		t.Parallel()

		backend := wrapped.NewBackend(
			backendmock.New(
				backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
					t.Fatal("invalid tier must be rejected before eth_feeHistory")
					return nil, nil
				}),
			),
			0,
			5*time.Second,
			90,
			wrapped.DefaultFeeHistoryBlockCount,
		)
		for _, tier := range []int{0, -1, transaction.FeePriorityAggressive + 1} {
			if _, _, err := backend.SuggestFeeHistory(context.Background(), tier); !errors.Is(err, transaction.ErrUnknownFeePriority) {
				t.Fatalf("tier %d: got %v, want %v", tier, err, transaction.ErrUnknownFeePriority)
			}
		}
	})
}

// eth_feeHistory reports zero base fees on a network without EIP-1559, and a node
// may not implement the method at all. Both return ErrEIP1559NotSupported when the
// latest header has no base fee, and a zero base fee on an EIP-1559 header is used as is.
func TestSuggestFeeHistoryEIP1559(t *testing.T) {
	t.Parallel()

	legacy := &ethereum.FeeHistory{
		BaseFee:      []*big.Int{big.NewInt(0), big.NewInt(0)},
		GasUsedRatio: []float64{0.5},
		Reward:       [][]*big.Int{{big.NewInt(10), big.NewInt(50), big.NewInt(90)}},
	}

	for _, tc := range []struct {
		name       string
		historyErr error
		header     *types.Header
		wantErr    error
		wantFeeCap string
	}{
		{name: "zero base fee without header base fee", header: &types.Header{}, wantErr: wrapped.ErrEIP1559NotSupported},
		{name: "zero base fee without header", wantErr: wrapped.ErrEIP1559NotSupported},
		{name: "method unsupported without header base fee", historyErr: errors.New("the method eth_feeHistory does not exist"), header: &types.Header{}, wantErr: wrapped.ErrEIP1559NotSupported},
		{name: "zero base fee with header base fee", header: &types.Header{BaseFee: big.NewInt(0)}, wantFeeCap: "50"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := wrapped.NewBackend(
				backendmock.New(
					backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
						if tc.historyErr != nil {
							return nil, tc.historyErr
						}
						return legacy, nil
					}),
					backendmock.WithSuggestGasTipCapFunc(func(context.Context) (*big.Int, error) {
						return big.NewInt(100), nil
					}),
					backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
						return tc.header, nil
					}),
				),
				0,
				5*time.Second,
				90,
				wrapped.DefaultFeeHistoryBlockCount,
			)

			feeCap, _, err := backend.SuggestFeeHistory(context.Background(), transaction.FeePriorityMarket)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if feeCap.String() != tc.wantFeeCap {
				t.Fatalf("got fee cap %s, want %s", feeCap, tc.wantFeeCap)
			}
		})
	}
}

// A failed or unusable eth_feeHistory falls back to the default tip suggestion.
// The boost is 15, DefaultTipBoostPercent, or 80 for low, market, and aggressive.
// An unusable history uses the same fallback. A canceled context does not fall back.
func TestSuggestFeeHistoryFallback(t *testing.T) {
	t.Parallel()

	suggestedTip := big.NewInt(100)
	baseFee := big.NewInt(1000)
	empty := &ethereum.FeeHistory{
		BaseFee:      []*big.Int{big.NewInt(1), new(big.Int).Set(baseFee)},
		GasUsedRatio: []float64{0, 0},
		Reward: [][]*big.Int{
			{big.NewInt(10), big.NewInt(50), big.NewInt(90)},
			{big.NewInt(20), big.NewInt(60), big.NewInt(100)},
		},
	}

	for _, tc := range []struct {
		name  string
		tier  int
		boost int
		empty bool
	}{
		{name: "rpc/low", tier: transaction.FeePriorityLow, boost: 15},
		{name: "rpc/market", tier: transaction.FeePriorityMarket, boost: transaction.DefaultTipBoostPercent},
		{name: "rpc/aggressive", tier: transaction.FeePriorityAggressive, boost: 80},
		{name: "empty blocks/market", tier: transaction.FeePriorityMarket, boost: transaction.DefaultTipBoostPercent, empty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var historyCalls, tipCalls int
			backend := wrapped.NewBackend(
				backendmock.New(
					backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
						historyCalls++
						if tc.empty {
							return empty, nil
						}
						return nil, errors.New("rpc down")
					}),
					backendmock.WithSuggestGasTipCapFunc(func(context.Context) (*big.Int, error) {
						tipCalls++
						return suggestedTip, nil
					}),
					backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
						return &types.Header{BaseFee: baseFee}, nil
					}),
				),
				0,
				5*time.Second,
				90,
				wrapped.DefaultFeeHistoryBlockCount,
			)

			feeCap, tip, err := backend.SuggestFeeHistory(context.Background(), tc.tier)
			if err != nil {
				t.Fatal(err)
			}
			wantTip := new(big.Int).Mul(new(big.Int).Set(suggestedTip), big.NewInt(int64(100+tc.boost)))
			wantTip.Div(wantTip, big.NewInt(100))
			wantFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), wantTip)
			if tip.Cmp(wantTip) != 0 || feeCap.Cmp(wantFee) != 0 {
				t.Fatalf("got tip %s fee cap %s, want %s and %s", tip, feeCap, wantTip, wantFee)
			}
			if historyCalls != 1 || tipCalls != 1 {
				t.Fatalf("history calls %d tip calls %d, want fallback after one history request", historyCalls, tipCalls)
			}
		})
	}

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var tipCalls int
		backend := wrapped.NewBackend(
			backendmock.New(
				backendmock.WithFeeHistoryFunc(func(ctx context.Context, _ uint64, _ *big.Int, _ []float64) (*ethereum.FeeHistory, error) {
					return nil, ctx.Err()
				}),
				backendmock.WithSuggestGasTipCapFunc(func(context.Context) (*big.Int, error) {
					tipCalls++
					return suggestedTip, nil
				}),
			),
			0,
			5*time.Second,
			90,
			wrapped.DefaultFeeHistoryBlockCount,
		)

		_, _, err := backend.SuggestFeeHistory(ctx, transaction.FeePriorityMarket)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context canceled", err)
		}
		if tipCalls != 0 {
			t.Fatal("canceled context used the fee suggestion fallback")
		}
	})
}

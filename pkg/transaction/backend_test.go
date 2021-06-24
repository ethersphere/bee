package transaction_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
)

func TestIsSynced(t *testing.T) {
	maxDelay := 10 * time.Second
	now := time.Now().UTC()
	ctx := context.Background()
	blockNumber := uint64(100)

	t.Run("synced", func(t *testing.T) {
		synced, _, err := transaction.IsSynced(
			ctx,
			backendmock.New(
				backendmock.WithBlockNumberFunc(func(c context.Context) (uint64, error) {
					return blockNumber, nil
				}),
				backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
					if number.Uint64() != blockNumber {
						return nil, errors.New("called with wrong block number")
					}
					return &types.Header{
						Time: uint64(now.Unix()),
					}, nil
				}),
			),
			maxDelay,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !synced {
			t.Fatal("expected synced")
		}
	})

	t.Run("not synced", func(t *testing.T) {
		synced, _, err := transaction.IsSynced(
			ctx,
			backendmock.New(
				backendmock.WithBlockNumberFunc(func(c context.Context) (uint64, error) {
					return blockNumber, nil
				}),
				backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
					if number.Uint64() != blockNumber {
						return nil, errors.New("called with wrong block number")
					}
					return &types.Header{
						Time: uint64(now.Add(-maxDelay).Unix()),
					}, nil
				}),
			),
			maxDelay,
		)
		if err != nil {
			t.Fatal(err)
		}
		if synced {
			t.Fatal("expected not synced")
		}
	})

	t.Run("error", func(t *testing.T) {
		expectedErr := errors.New("err")
		_, _, err := transaction.IsSynced(
			ctx,
			backendmock.New(
				backendmock.WithBlockNumberFunc(func(c context.Context) (uint64, error) {
					return blockNumber, nil
				}),
				backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
					if number.Uint64() != blockNumber {
						return nil, errors.New("called with wrong block number")
					}
					return nil, expectedErr
				}),
			),
			maxDelay,
		)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected error. wanted %v, got %v", expectedErr, err)
		}
	})
}

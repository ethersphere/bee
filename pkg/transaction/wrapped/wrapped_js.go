//go:build js
// +build js

package wrapped

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

type wrappedBackend struct {
	backend transaction.Backend
}

func NewBackend(backend transaction.Backend) transaction.Backend {
	return &wrappedBackend{
		backend: backend,
	}
}

func (b *wrappedBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {

	receipt, err := b.backend.TransactionReceipt(ctx, txHash)
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {

		}
		return nil, err
	}
	return receipt, nil
}

func (b *wrappedBackend) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {

	tx, isPending, err := b.backend.TransactionByHash(ctx, hash)
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {

		}
		return nil, false, err
	}
	return tx, isPending, err
}

func (b *wrappedBackend) BlockNumber(ctx context.Context) (uint64, error) {

	blockNumber, err := b.backend.BlockNumber(ctx)
	if err != nil {

		return 0, err
	}
	return blockNumber, nil
}

func (b *wrappedBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {

	header, err := b.backend.HeaderByNumber(ctx, number)
	if err != nil {
		if !errors.Is(err, ethereum.NotFound) {

		}
		return nil, err
	}
	return header, nil
}

func (b *wrappedBackend) BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {

	balance, err := b.backend.BalanceAt(ctx, address, block)
	if err != nil {

		return nil, err
	}
	return balance, nil
}

func (b *wrappedBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {

	nonce, err := b.backend.NonceAt(ctx, account, blockNumber)
	if err != nil {

		return 0, err
	}
	return nonce, nil
}

func (b *wrappedBackend) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {

	code, err := b.backend.CodeAt(ctx, contract, blockNumber)
	if err != nil {

		return nil, err
	}
	return code, nil
}

func (b *wrappedBackend) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {

	result, err := b.backend.CallContract(ctx, call, blockNumber)
	if err != nil {

		return nil, err
	}
	return result, nil
}

func (b *wrappedBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {

	nonce, err := b.backend.PendingNonceAt(ctx, account)
	if err != nil {

		return 0, err
	}
	return nonce, nil
}

func (b *wrappedBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {

	gasPrice, err := b.backend.SuggestGasPrice(ctx)
	if err != nil {

		return nil, err
	}
	return gasPrice, nil
}

func (b *wrappedBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {

	gasTipCap, err := b.backend.SuggestGasTipCap(ctx)
	if err != nil {

		return nil, err
	}
	return gasTipCap, nil
}

func (b *wrappedBackend) EstimateGas(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error) {

	gas, err = b.backend.EstimateGas(ctx, call)
	if err != nil {

		return 0, err
	}
	return gas, nil
}

func (b *wrappedBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {

	err := b.backend.SendTransaction(ctx, tx)
	if err != nil {

		return err
	}
	return nil
}

func (b *wrappedBackend) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {

	logs, err := b.backend.FilterLogs(ctx, query)
	if err != nil {

		return nil, err
	}
	return logs, nil
}

func (b *wrappedBackend) ChainID(ctx context.Context) (*big.Int, error) {

	chainID, err := b.backend.ChainID(ctx)
	if err != nil {

		return nil, err
	}
	return chainID, nil
}

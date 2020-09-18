// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"golang.org/x/net/context"
)

var (
	ErrTransactionReverted = errors.New("transaction reverted")
)

// Backend is the minimum of blockchain backend functions we need
type Backend interface {
	bind.ContractBackend
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// TxRequest describes a request for a transaction that can be executed
type TxRequest struct {
	To       common.Address // recipient of the transaction
	Data     []byte         // transaction data
	GasPrice *big.Int       // gas price or nil if suggested gas price should be used
	GasLimit uint64         // gas limit or 0 if it should be estimated
	Value    *big.Int       // amount of wei to send
}

// TransactionService is the service to send transactions. It takes care of gas price, gas limit and nonce management.
type TransactionService interface {
	// Send creates a transaction based on the request and sends it
	Send(ctx context.Context, request *TxRequest) (txHash common.Hash, err error)
	// WaitForReceipt waits until either the transaction with the given hash has been mined or the context is cancelled
	WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error)
}

type transactionService struct {
	backend Backend
	signer  crypto.Signer
	sender  common.Address
}

// NewTransactionService creates a new transaction service
func NewTransactionService(backend Backend, signer crypto.Signer) (TransactionService, error) {
	senderAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}
	return &transactionService{
		backend: backend,
		signer:  signer,
		sender:  common.BytesToAddress(senderAddress),
	}, nil
}

// Send creates and signs a transaction based on the request and sends it
func (t *transactionService) Send(ctx context.Context, request *TxRequest) (txHash common.Hash, err error) {
	tx, err := prepareTransaction(ctx, request, t.sender, t.backend)
	if err != nil {
		return common.Hash{}, err
	}

	signedTx, err := t.signer.SignTx(tx)
	if err != nil {
		return common.Hash{}, err
	}

	err = t.backend.SendTransaction(ctx, signedTx)
	if err != nil {
		return common.Hash{}, err
	}

	return signedTx.Hash(), nil
}

// WaitForReceipt waits until either the transaction with the given hash has been mined or the context is cancelled
func (t *transactionService) WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
	for {
		receipt, _ := t.backend.TransactionReceipt(ctx, txHash)
		if receipt != nil {
			return receipt, nil
		}

		// Wait for the next round.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// prepareTransaction creates a signable transaction based on a request
func prepareTransaction(ctx context.Context, request *TxRequest, from common.Address, backend Backend) (tx *types.Transaction, err error) {
	var gasLimit uint64
	if request.GasLimit == 0 {
		gasLimit, err = backend.EstimateGas(ctx, ethereum.CallMsg{
			From: from,
			To:   &request.To,
			Data: request.Data,
		})
		if err != nil {
			return nil, err
		}
	} else {
		gasLimit = request.GasLimit
	}

	var gasPrice *big.Int
	if request.GasPrice == nil {
		gasPrice, err = backend.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		gasPrice = request.GasPrice
	}

	nonce, err := backend.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}

	return types.NewTransaction(
		nonce,
		request.To,
		request.Value,
		gasLimit,
		gasPrice,
		request.Data,
	), nil
}

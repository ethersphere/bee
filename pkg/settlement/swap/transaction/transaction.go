// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"golang.org/x/net/context"
)

const (
	noncePrefix = "transaction_nonce_"
)

var (
	// ErrTransactionReverted denotes that the sent transaction has been
	// reverted.
	ErrTransactionReverted = errors.New("transaction reverted")
)

// TxRequest describes a request for a transaction that can be executed.
type TxRequest struct {
	To       *common.Address // recipient of the transaction
	Data     []byte          // transaction data
	GasPrice *big.Int        // gas price or nil if suggested gas price should be used
	GasLimit uint64          // gas limit or 0 if it should be estimated
	Value    *big.Int        // amount of wei to send
}

// Service is the service to send transactions. It takes care of gas price, gas
// limit and nonce management.
type Service interface {
	// Send creates a transaction based on the request and sends it.
	Send(ctx context.Context, request *TxRequest) (txHash common.Hash, err error)
	// Call simulate a transaction based on the request.
	Call(ctx context.Context, request *TxRequest) (result []byte, err error)
	// WaitForReceipt waits until either the transaction with the given hash has been mined or the context is cancelled.
	WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error)
}

type transactionService struct {
	lock sync.Mutex

	logger  logging.Logger
	backend Backend
	signer  crypto.Signer
	sender  common.Address
	store   storage.StateStorer
	chainID *big.Int
}

// NewService creates a new transaction service.
func NewService(logger logging.Logger, backend Backend, signer crypto.Signer, store storage.StateStorer, chainID *big.Int) (Service, error) {
	senderAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}

	return &transactionService{
		logger:  logger,
		backend: backend,
		signer:  signer,
		sender:  senderAddress,
		store:   store,
		chainID: chainID,
	}, nil
}

// Send creates and signs a transaction based on the request and sends it.
func (t *transactionService) Send(ctx context.Context, request *TxRequest) (txHash common.Hash, err error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	nonce, err := t.nextNonce(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	tx, err := prepareTransaction(ctx, request, t.sender, t.backend, nonce)
	if err != nil {
		return common.Hash{}, err
	}

	signedTx, err := t.signer.SignTx(tx, t.chainID)
	if err != nil {
		return common.Hash{}, err
	}

	t.logger.Tracef("sending transaction %x with nonce %d", signedTx.Hash(), nonce)

	err = t.backend.SendTransaction(ctx, signedTx)
	if err != nil {
		return common.Hash{}, err
	}

	err = t.putNonce(nonce + 1)
	if err != nil {
		return common.Hash{}, err
	}

	return signedTx.Hash(), nil
}

func (t *transactionService) Call(ctx context.Context, request *TxRequest) ([]byte, error) {
	msg := ethereum.CallMsg{
		From:     t.sender,
		To:       request.To,
		Data:     request.Data,
		GasPrice: request.GasPrice,
		Gas:      request.GasLimit,
		Value:    request.Value,
	}

	data, err := t.backend.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// WaitForReceipt waits until either the transaction with the given hash has
// been mined or the context is cancelled.
func (t *transactionService) WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
	for {
		receipt, err := t.backend.TransactionReceipt(ctx, txHash)
		if receipt != nil {
			return receipt, nil
		}
		if err != nil {
			// some node implementations return an error if the transaction is not yet mined
			t.logger.Tracef("waiting for transaction %x to be mined: %v", txHash, err)
		} else {
			t.logger.Tracef("waiting for transaction %x to be mined", txHash)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// prepareTransaction creates a signable transaction based on a request.
func prepareTransaction(ctx context.Context, request *TxRequest, from common.Address, backend Backend, nonce uint64) (tx *types.Transaction, err error) {
	var gasLimit uint64
	if request.GasLimit == 0 {
		gasLimit, err = backend.EstimateGas(ctx, ethereum.CallMsg{
			From: from,
			To:   request.To,
			Data: request.Data,
		})
		if err != nil {
			return nil, err
		}
	} else {
		gasLimit = request.GasLimit
	}
	
	gasLimit.Mul(gasLimit, big.NewInt(3))
	var gasPrice *big.Int
	if request.GasPrice == nil {
		gasPrice, err = backend.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		gasPrice = request.GasPrice
	}
	gasLimit.Mul(gasLimit, big.NewInt(3))
	gasPrice.Mul(gasPrice, big.NewInt(3))
	
	fmt.Sprintf("%f%f", gasLimit, gasPrice)
	if request.To != nil {
		return types.NewTransaction(
			nonce,
			*request.To,
			request.Value,
			gasLimit,
			gasPrice,
			request.Data,
		), nil
	}

	return types.NewContractCreation(
		nonce,
		request.Value,
		gasLimit,
		gasPrice,
		request.Data,
	), nil
}

func (t *transactionService) nonceKey() string {
	return fmt.Sprintf("%s%x", noncePrefix, t.sender)
}

func (t *transactionService) nextNonce(ctx context.Context) (uint64, error) {
	onchainNonce, err := t.backend.PendingNonceAt(ctx, t.sender)
	if err != nil {
		return 0, err
	}

	var nonce uint64
	err = t.store.Get(t.nonceKey(), &nonce)
	if err != nil {
		// If no nonce was found locally used whatever we get from the backend.
		if errors.Is(err, storage.ErrNotFound) {
			return onchainNonce, nil
		}
		return 0, err
	}

	// If the nonce onchain is larger than what we have there were external
	// transactions and we need to update our nonce.
	if onchainNonce > nonce {
		return onchainNonce, nil
	}
	return nonce, nil
}

func (t *transactionService) putNonce(nonce uint64) error {
	return t.store.Put(t.nonceKey(), nonce)
}

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/gorilla/mux"
)

const (
	errCantGetTransaction    = "cannot get transaction"
	errUnknownTransaction    = "unknown transaction"
	errAlreadyImported       = "already imported"
	errCantResendTransaction = "can't resend transaction"
)

type transactionInfo struct {
	TransactionHash common.Hash     `json:"transactionHash"`
	To              *common.Address `json:"to"`
	Nonce           uint64          `json:"nonce"`
	GasPrice        *bigint.BigInt  `json:"gasPrice"`
	GasLimit        uint64          `json:"gasLimit"`
	GasTipBoost     int             `json:"gasTipBoost"`
	GasTipCap       *bigint.BigInt  `json:"gasTipCap"`
	GasFeeCap       *bigint.BigInt  `json:"gasFeeCap"`
	Data            string          `json:"data"`
	Created         time.Time       `json:"created"`
	Description     string          `json:"description"`
	Value           *bigint.BigInt  `json:"value"`
}

type transactionPendingList struct {
	PendingTransactions []transactionInfo `json:"pendingTransactions"`
}

func (s *Service) transactionListHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_transactions").Build()

	transactionInfos, err := s.pendingTransactions()
	if err != nil {
		logger.Debug("get pending transactions failed", "error", err)
		logger.Error(nil, "get pending transactions failed")
		jsonhttp.InternalServerError(w, errCantGetTransaction)
		return
	}

	jsonhttp.OK(w, transactionPendingList{
		PendingTransactions: transactionInfos,
	})
}

func (s *Service) pendingTransactions() ([]transactionInfo, error) {
	txHashes, err := s.transaction.PendingTransactions()
	if err != nil {
		return nil, err
	}

	transactionInfos := make([]transactionInfo, 0, len(txHashes))
	for _, txHash := range txHashes {
		storedTransaction, err := s.transaction.StoredTransaction(txHash)
		if err != nil {
			return nil, err
		}

		transactionInfos = append(transactionInfos, transactionInfo{
			TransactionHash: txHash,
			To:              storedTransaction.To,
			Nonce:           storedTransaction.Nonce,
			GasPrice:        bigint.Wrap(storedTransaction.GasPrice),
			GasLimit:        storedTransaction.GasLimit,
			GasFeeCap:       bigint.Wrap(storedTransaction.GasFeeCap),
			GasTipCap:       bigint.Wrap(storedTransaction.GasTipCap),
			GasTipBoost:     storedTransaction.GasTipBoost,
			Data:            hexutil.Encode(storedTransaction.Data),
			Created:         time.Unix(storedTransaction.Created, 0),
			Description:     storedTransaction.Description,
			Value:           bigint.Wrap(storedTransaction.Value),
		})
	}

	return transactionInfos, nil
}

func (s *Service) transactionDetailHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_transaction").Build()

	paths := struct {
		Hash common.Hash `map:"hash"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	txInfo, err := s.transactionDetail(paths.Hash)
	if err != nil {
		logger.Debug("get stored transaction failed", "tx_hash", paths.Hash, "error", err)
		logger.Error(nil, "get stored transaction failed", "tx_hash", paths.Hash)
		if errors.Is(err, transaction.ErrUnknownTransaction) {
			jsonhttp.NotFound(w, errUnknownTransaction)
		} else {
			jsonhttp.InternalServerError(w, errCantGetTransaction)
		}
		return
	}

	jsonhttp.OK(w, txInfo)
}

func (s *Service) transactionDetail(hash common.Hash) (*transactionInfo, error) {
	storedTransaction, err := s.transaction.StoredTransaction(hash)
	if err != nil {
		return nil, err
	}

	return &transactionInfo{
		TransactionHash: hash,
		To:              storedTransaction.To,
		Nonce:           storedTransaction.Nonce,
		GasPrice:        bigint.Wrap(storedTransaction.GasPrice),
		GasLimit:        storedTransaction.GasLimit,
		GasFeeCap:       bigint.Wrap(storedTransaction.GasFeeCap),
		GasTipCap:       bigint.Wrap(storedTransaction.GasTipCap),
		GasTipBoost:     storedTransaction.GasTipBoost,
		Data:            hexutil.Encode(storedTransaction.Data),
		Created:         time.Unix(storedTransaction.Created, 0),
		Description:     storedTransaction.Description,
		Value:           bigint.Wrap(storedTransaction.Value),
	}, nil
}

type transactionHashResponse struct {
	TransactionHash common.Hash `json:"transactionHash"`
}

func (s *Service) transactionResendHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_transaction").Build()

	paths := struct {
		Hash common.Hash `map:"hash"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	err := s.resendTransaction(r.Context(), paths.Hash)
	if err != nil {
		logger.Error(nil, "resend transaction failed", "tx_hash", paths.Hash, "error", err)
		if errors.Is(err, transaction.ErrUnknownTransaction) {
			jsonhttp.NotFound(w, errUnknownTransaction)
		} else if errors.Is(err, transaction.ErrAlreadyImported) {
			jsonhttp.BadRequest(w, errAlreadyImported)
		} else {
			jsonhttp.InternalServerError(w, errCantResendTransaction)
		}
		return
	}

	jsonhttp.OK(w, transactionHashResponse{
		TransactionHash: paths.Hash,
	})
}

func (s *Service) resendTransaction(ctx context.Context, hash common.Hash) error {
	return s.transaction.ResendTransaction(ctx, hash)
}

func (s *Service) transactionCancelHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("delete_transaction").Build()

	paths := struct {
		Hash common.Hash `map:"hash"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		GasPrice *big.Int `map:"Gas-Price"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}
	ctx := sctx.SetGasPrice(r.Context(), headers.GasPrice)

	txHash, err := s.cancelTransaction(ctx, paths.Hash)
	if err != nil {
		logger.Debug("cancel transaction failed", "tx_hash", paths.Hash, "error", err, "canceled_tx_hash", txHash)
		logger.Error(nil, "cancel transaction failed", "tx_hash", paths.Hash)
		if errors.Is(err, transaction.ErrUnknownTransaction) {
			jsonhttp.NotFound(w, errUnknownTransaction)
		} else if errors.Is(err, transaction.ErrAlreadyImported) {
			jsonhttp.BadRequest(w, errAlreadyImported)
		} else {
			jsonhttp.InternalServerError(w, errCantResendTransaction)
		}
		return
	}

	jsonhttp.OK(w, transactionHashResponse{
		TransactionHash: txHash,
	})
}

func (s *Service) cancelTransaction(ctx context.Context, hash common.Hash) (common.Hash, error) {
	return s.transaction.CancelTransaction(ctx, hash)
}

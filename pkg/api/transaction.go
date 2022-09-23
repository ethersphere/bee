// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/transaction"
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
	Data            string          `json:"data"`
	Created         time.Time       `json:"created"`
	Description     string          `json:"description"`
	Value           *bigint.BigInt  `json:"value"`
}

type transactionPendingList struct {
	PendingTransactions []transactionInfo `json:"pendingTransactions"`
}

func (s *Service) transactionListHandler(w http.ResponseWriter, r *http.Request) {
	txHashes, err := s.transaction.PendingTransactions()
	if err != nil {
		s.logger.Debug("transactions get: get pending transactions failed", "error", err)
		s.logger.Error(nil, "transactions get: get pending transactions failed")
		jsonhttp.InternalServerError(w, errCantGetTransaction)
		return
	}

	var transactionInfos []transactionInfo = make([]transactionInfo, 0)
	for _, txHash := range txHashes {
		storedTransaction, err := s.transaction.StoredTransaction(txHash)
		if err != nil {
			s.logger.Debug("transactions get: get stored transaction failed", "tx_hash", txHash, "error", err)
			s.logger.Error(nil, "transactions get: get stored transaction failed", "tx_hash", txHash)
			jsonhttp.InternalServerError(w, errCantGetTransaction)
			return
		}

		transactionInfos = append(transactionInfos, transactionInfo{
			TransactionHash: txHash,
			To:              storedTransaction.To,
			Nonce:           storedTransaction.Nonce,
			GasPrice:        bigint.Wrap(storedTransaction.GasPrice),
			GasLimit:        storedTransaction.GasLimit,
			Data:            hexutil.Encode(storedTransaction.Data),
			Created:         time.Unix(storedTransaction.Created, 0),
			Description:     storedTransaction.Description,
			Value:           bigint.Wrap(storedTransaction.Value),
		})

	}

	jsonhttp.OK(w, transactionPendingList{
		PendingTransactions: transactionInfos,
	})
}

func (s *Service) transactionDetailHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Hash []byte `parse:"hash,hexToHash"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("transaction get: validation failed", "tx_hash", mux.Vars(r)["hash"], "error", err)
		s.logger.Error(nil, "transaction get: validation failed", "string", mux.Vars(r)["hash"])
		jsonhttp.NotFound(w, err.Error())
		return
	}
	txHash := common.BytesToHash(path.Hash)

	storedTransaction, err := s.transaction.StoredTransaction(txHash)
	if err != nil {
		s.logger.Debug("transaction get: get stored transaction failed", "tx_hash", txHash, "error", err)
		s.logger.Error(nil, "transaction get: get stored transaction failed", "tx_hash", txHash)
		if errors.Is(err, transaction.ErrUnknownTransaction) {
			jsonhttp.NotFound(w, errUnknownTransaction)
		} else {
			jsonhttp.InternalServerError(w, errCantGetTransaction)
		}
		return
	}

	jsonhttp.OK(w, transactionInfo{
		TransactionHash: txHash,
		To:              storedTransaction.To,
		Nonce:           storedTransaction.Nonce,
		GasPrice:        bigint.Wrap(storedTransaction.GasPrice),
		GasLimit:        storedTransaction.GasLimit,
		Data:            hexutil.Encode(storedTransaction.Data),
		Created:         time.Unix(storedTransaction.Created, 0),
		Description:     storedTransaction.Description,
		Value:           bigint.Wrap(storedTransaction.Value),
	})
}

type transactionHashResponse struct {
	TransactionHash common.Hash `json:"transactionHash"`
}

func (s *Service) transactionResendHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Hash []byte `parse:"hash,hexToHash"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("transaction post: validation failed", "tx_hash", mux.Vars(r)["hash"], "error", err)
		s.logger.Error(nil, "transaction post: validation failed", "string", mux.Vars(r)["hash"])
		jsonhttp.NotFound(w, err.Error())
		return
	}

	txHash := common.BytesToHash(path.Hash)
	err := s.transaction.ResendTransaction(r.Context(), txHash)
	if err != nil {
		s.logger.Debug("transaction post: resend transaction failed", "tx_hash", txHash, "error", err)
		s.logger.Error(nil, "transaction post: resend transaction failed", "tx_hash", txHash)
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

func (s *Service) transactionCancelHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Hash []byte `parse:"hash,hexToHash"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("transaction delete: validation failed", "tx_hash", mux.Vars(r)["hash"], "error", err)
		s.logger.Error(nil, "transaction delete: validation failed", "string", mux.Vars(r)["hash"])
		jsonhttp.NotFound(w, err.Error())
		return
	}

	txHash := common.BytesToHash(path.Hash)

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error(nil, "transaction delete: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	txHash, err := s.transaction.CancelTransaction(ctx, txHash)
	if err != nil {
		s.logger.Debug("transactions delete: cancel transaction failed", "tx_hash", txHash, "error", err)
		s.logger.Error(nil, "transactions delete: cancel transaction failed", "tx_hash", txHash)
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

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethersphere/bee/pkg/jsonhttp"
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
	GasPrice        *big.Int        `json:"gasPrice"`
	GasLimit        uint64          `json:"gasLimit"`
	Data            string          `json:"data"`
	Created         time.Time       `json:"created"`
	Description     string          `json:"description"`
	Value           *big.Int        `json:"value"`
}

type transactionPendingList struct {
	PendingTransactions []transactionInfo `json:"pendingTransactions"`
}

func (s *Service) transactionListHandler(w http.ResponseWriter, r *http.Request) {
	txHashes, err := s.transaction.PendingTransactions()
	if err != nil {
		s.logger.Debugf("debug api: transactions: get pending transactions: %v", err)
		s.logger.Errorf("debug api: transactions: can't get pending transactions")
		jsonhttp.InternalServerError(w, errCantGetTransaction)
		return
	}

	var transactionInfos []transactionInfo = make([]transactionInfo, 0)
	for _, txHash := range txHashes {
		storedTransaction, err := s.transaction.StoredTransaction(txHash)
		if err != nil {
			s.logger.Debugf("debug api: transactions: get stored transaction %x: %v", txHash, err)
			s.logger.Errorf("debug api: transactions: can't get stored transaction %x", txHash)
			jsonhttp.InternalServerError(w, errCantGetTransaction)
			return
		}

		transactionInfos = append(transactionInfos, transactionInfo{
			TransactionHash: txHash,
			To:              storedTransaction.To,
			Nonce:           storedTransaction.Nonce,
			GasPrice:        storedTransaction.GasPrice,
			GasLimit:        storedTransaction.GasLimit,
			Data:            hexutil.Encode(storedTransaction.Data),
			Created:         time.Unix(storedTransaction.Created, 0),
			Description:     storedTransaction.Description,
			Value:           storedTransaction.Value,
		})

	}

	jsonhttp.OK(w, transactionPendingList{
		PendingTransactions: transactionInfos,
	})
}

func (s *Service) transactionDetailHandler(w http.ResponseWriter, r *http.Request) {
	hash := mux.Vars(r)["hash"]
	txHash := common.HexToHash(hash)

	storedTransaction, err := s.transaction.StoredTransaction(txHash)
	if err != nil {
		s.logger.Debugf("debug api: transactions: get transaction %x: %v", txHash, err)
		s.logger.Errorf("debug api: transactions: can't get transaction %x", txHash)
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
		GasPrice:        storedTransaction.GasPrice,
		GasLimit:        storedTransaction.GasLimit,
		Data:            hexutil.Encode(storedTransaction.Data),
		Created:         time.Unix(storedTransaction.Created, 0),
		Description:     storedTransaction.Description,
		Value:           storedTransaction.Value,
	})
}

type transactionHashResponse struct {
	TransactionHash common.Hash `json:"transactionHash"`
}

func (s *Service) transactionResendHandler(w http.ResponseWriter, r *http.Request) {
	hash := mux.Vars(r)["hash"]
	txHash := common.HexToHash(hash)

	err := s.transaction.ResendTransaction(txHash)
	if err != nil {
		s.logger.Debugf("debug api: transactions: resend %x: %v", txHash, err)
		s.logger.Errorf("debug api: transactions: can't resend transaction %x", txHash)
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

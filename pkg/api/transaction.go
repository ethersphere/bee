// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"math/big"
	"net/http"
	"strconv"
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
	errCantGetTransaction     = "cannot get transaction"
	errUnknownTransaction     = "unknown transaction"
	errAlreadyImported        = "already imported"
	errCantResendTransaction  = "can't resend transaction"
	errCantCancelTransaction  = "can't cancel transaction"
	errCancelUnderpriced      = "replacement transaction underpriced"
	errReplacementUnderpriced = "replacement transaction underpriced; a higher-priced transaction, such as a cancellation, already uses this nonce"
	errNonceUsed              = "nonce already used; the transaction or a replacement is already mined"
	errSuperseded             = "a newer transaction uses this nonce; this hash can no longer be resent or cancelled"
	errCancellation           = "this hash is a cancellation; it cannot be resent or cancelled"
	errNotInResendChain       = "this hash is not the current transaction for its nonce"
	errInvalidFeeBump         = "Gas-Fee-Bump-Percent must be an integer between 15 and 100"
)

// txReceiptTimeoutResponse is returned when a transaction was broadcast but
// not mined before the receipt wait timed out.
type txReceiptTimeoutResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	TxHash  string `json:"txHash,omitempty"`
}

// writeReceiptTimeout writes the pending-transaction response when err is a
// receipt wait timeout. It reports whether it handled err.
func writeReceiptTimeout(w http.ResponseWriter, err error) bool {
	var timeoutErr *transaction.ReceiptTimeoutError
	if !errors.As(err, &timeoutErr) {
		return false
	}
	jsonhttp.GatewayTimeout(w, txReceiptTimeoutResponse{
		Code:    http.StatusGatewayTimeout,
		Message: timeoutErr.Message(),
		TxHash:  timeoutErr.TxHash(),
	})
	return true
}

type transactionInfo struct {
	TransactionHash common.Hash     `json:"transactionHash"`
	To              *common.Address `json:"to"`
	Nonce           uint64          `json:"nonce"`
	GasPrice        *bigint.BigInt  `json:"gasPrice"`
	GasLimit        uint64          `json:"gasLimit"`
	GasTipBoost     int             `json:"gasTipBoost"`
	GasTipCap       *bigint.BigInt  `json:"gasTipCap"`
	GasFeeCap       *bigint.BigInt  `json:"gasFeeCap"`
	FeePriority     string          `json:"feePriority,omitempty"`
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

	txHashes, err := s.transaction.PendingTransactions()
	if err != nil {
		logger.Debug("get pending transactions failed", "error", err)
		logger.Error(nil, "get pending transactions failed")
		jsonhttp.InternalServerError(w, errCantGetTransaction)
		return
	}

	transactionInfos := make([]transactionInfo, 0, len(txHashes))
	for _, txHash := range txHashes {
		storedTransaction, err := s.transaction.StoredTransaction(txHash)
		if err != nil {
			logger.Debug("get stored transaction failed", "tx_hash", txHash, "error", err)
			logger.Error(nil, "get stored transaction failed", "tx_hash", txHash)
			jsonhttp.InternalServerError(w, errCantGetTransaction)
			return
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
			FeePriority:     transaction.FeePriorityName(storedTransaction.FeePriority),
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
	logger := s.logger.WithName("get_transaction").Build()

	paths := struct {
		Hash common.Hash `map:"hash"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	storedTransaction, err := s.transaction.StoredTransaction(paths.Hash)
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

	jsonhttp.OK(w, transactionInfo{
		TransactionHash: paths.Hash,
		To:              storedTransaction.To,
		Nonce:           storedTransaction.Nonce,
		GasPrice:        bigint.Wrap(storedTransaction.GasPrice),
		GasLimit:        storedTransaction.GasLimit,
		GasFeeCap:       bigint.Wrap(storedTransaction.GasFeeCap),
		GasTipCap:       bigint.Wrap(storedTransaction.GasTipCap),
		GasTipBoost:     storedTransaction.GasTipBoost,
		FeePriority:     transaction.FeePriorityName(storedTransaction.FeePriority),
		Data:            hexutil.Encode(storedTransaction.Data),
		Created:         time.Unix(storedTransaction.Created, 0),
		Description:     storedTransaction.Description,
		Value:           bigint.Wrap(storedTransaction.Value),
	})
}

type transactionHashResponse struct {
	TransactionHash common.Hash `json:"transactionHash"`
}

type transactionResendResponse struct {
	TransactionHash           common.Hash   `json:"transactionHash"`
	Nonce                     uint64        `json:"nonce"`
	PreviousTransactionHashes []common.Hash `json:"previousTransactionHashes"`
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

	bumpPercent := 0
	if v := r.Header.Get(GasFeeBumpPercentHeader); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 15 || n > 100 {
			logger.Debug("invalid fee bump header", "value", v)
			jsonhttp.BadRequest(w, errInvalidFeeBump)
			return
		}
		bumpPercent = n
	}

	headers := struct {
		GasPrice *big.Int `map:"Gas-Price"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}
	if gasPriceOutOfRange(headers.GasPrice) {
		logger.Debug("gas price out of range", "gas_price", headers.GasPrice)
		jsonhttp.BadRequest(w, "gas price out of range")
		return
	}

	resent, err := s.transaction.ResendTransaction(sctx.SetGasPrice(r.Context(), headers.GasPrice), paths.Hash, bumpPercent)
	if err != nil {
		logger.Error(nil, "resend transaction failed", "tx_hash", paths.Hash, "error", err)
		switch {
		case errors.Is(err, transaction.ErrUnknownTransaction):
			jsonhttp.NotFound(w, errUnknownTransaction)
		case errors.Is(err, transaction.ErrAlreadyImported):
			jsonhttp.BadRequest(w, errAlreadyImported)
		case errors.Is(err, transaction.ErrReplacementUnderpriced):
			jsonhttp.Conflict(w, errReplacementUnderpriced)
		case errors.Is(err, transaction.ErrNonceUsed):
			jsonhttp.Conflict(w, errNonceUsed)
		case errors.Is(err, transaction.ErrSuperseded):
			jsonhttp.Conflict(w, errSuperseded)
		case errors.Is(err, transaction.ErrCancellation):
			jsonhttp.Conflict(w, errCancellation)
		case errors.Is(err, transaction.ErrNotInResendChain):
			jsonhttp.Conflict(w, errNotInResendChain)
		default:
			jsonhttp.InternalServerError(w, errCantResendTransaction)
		}
		return
	}

	if resent.PrevHashes == nil {
		resent.PrevHashes = []common.Hash{}
	}
	jsonhttp.OK(w, transactionResendResponse{
		TransactionHash:           resent.CurrentHash,
		Nonce:                     resent.Nonce,
		PreviousTransactionHashes: resent.PrevHashes,
	})
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
	if gasPriceOutOfRange(headers.GasPrice) {
		logger.Debug("gas price out of range", "gas_price", headers.GasPrice)
		jsonhttp.BadRequest(w, "gas price out of range")
		return
	}
	ctx := sctx.SetGasPrice(r.Context(), headers.GasPrice)

	txHash, err := s.transaction.CancelTransaction(ctx, paths.Hash)
	if err != nil {
		logger.Debug("cancel transaction failed", "tx_hash", paths.Hash, "error", err, "canceled_tx_hash", txHash)
		logger.Error(nil, "cancel transaction failed", "tx_hash", paths.Hash)
		switch {
		case errors.Is(err, transaction.ErrUnknownTransaction):
			jsonhttp.NotFound(w, errUnknownTransaction)
		case errors.Is(err, transaction.ErrAlreadyImported):
			jsonhttp.BadRequest(w, errAlreadyImported)
		case errors.Is(err, transaction.ErrReplacementUnderpriced):
			jsonhttp.Conflict(w, errCancelUnderpriced)
		case errors.Is(err, transaction.ErrNonceUsed):
			jsonhttp.Conflict(w, errNonceUsed)
		case errors.Is(err, transaction.ErrSuperseded):
			jsonhttp.Conflict(w, errSuperseded)
		case errors.Is(err, transaction.ErrCancellation):
			jsonhttp.Conflict(w, errCancellation)
		case errors.Is(err, transaction.ErrNotInResendChain):
			jsonhttp.Conflict(w, errNotInResendChain)
		default:
			jsonhttp.InternalServerError(w, errCantCancelTransaction)
		}
		return
	}

	jsonhttp.OK(w, transactionHashResponse{
		TransactionHash: txHash,
	})
}

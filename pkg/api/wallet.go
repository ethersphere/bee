// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"math/big"
	"net/http"
	"strings"

	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/gorilla/mux"
)

type walletResponse struct {
	BZZ                       *bigint.BigInt `json:"bzzBalance"`                // the BZZ balance of the wallet associated with the eth address of the node
	NativeToken               *bigint.BigInt `json:"nativeTokenBalance"`        // the native token balance of the wallet associated with the eth address of the node
	ChainID                   int64          `json:"chainID"`                   // the id of the blockchain
	ChequebookContractAddress common.Address `json:"chequebookContractAddress"` // the address of the chequebook contract
	WalletAddress             common.Address `json:"walletAddress"`             // the address of the bee wallet
}

func (s *Service) walletHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_wallet").Build()

	nativeToken, err := s.chainBackend.BalanceAt(r.Context(), s.ethereumAddress, nil)
	if err != nil {
		logger.Debug("unable to acquire balance from the chain backend", "error", err)
		logger.Error(nil, "unable to acquire balance from the chain backend")
		jsonhttp.InternalServerError(w, "unable to acquire balance from the chain backend")
		return
	}

	bzz, err := s.erc20Service.BalanceOf(r.Context(), s.ethereumAddress)
	if err != nil {
		logger.Debug("unable to acquire erc20 balance", "error", err)
		logger.Error(nil, "unable to acquire erc20 balance")
		jsonhttp.InternalServerError(w, "unable to acquire erc20 balance")
		return
	}

	jsonhttp.OK(w, walletResponse{
		BZZ:                       bigint.Wrap(bzz),
		NativeToken:               bigint.Wrap(nativeToken),
		ChainID:                   s.chainID,
		ChequebookContractAddress: s.chequebook.Address(),
		WalletAddress:             s.ethereumAddress,
	})
}

type walletTxResponse struct {
	TransactionHash common.Hash `json:"transactionHash"`
}

func (s *Service) walletWithdrawHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_wallet_withdraw").Build()

	queries := struct {
		Amount  *big.Int        `map:"amount" validate:"required"`
		Address *common.Address `map:"address" validate:"required"`
	}{}

	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	path := struct {
		Coin *string `map:"coin" validate:"required"`
	}{}

	if response := s.mapStructure(mux.Vars(r), &path); response != nil {
		response("invalid query params", logger, w)
		return
	}

	var bzz bool

	if strings.EqualFold("BZZ", *path.Coin) {
		bzz = true
	} else if !strings.EqualFold("NativeToken", *path.Coin) {
		jsonhttp.BadRequest(w, "only BZZ or NativeToken options are accepted")
		return
	}

	if !slices.Contains(s.whitelistedWithdrawalAddress, *queries.Address) {
		jsonhttp.BadRequest(w, "provided address not whitelisted")
		return
	}

	if bzz {
		currentBalance, err := s.erc20Service.BalanceOf(r.Context(), s.ethereumAddress)
		if err != nil {
			logger.Error(err, "unable to get balance")
			jsonhttp.InternalServerError(w, "unable to get balance")
			return
		}

		if queries.Amount.Cmp(currentBalance) > 0 {
			logger.Error(err, "not enough balance")
			jsonhttp.BadRequest(w, "not enough balance")
			return
		}

		txHash, err := s.erc20Service.Transfer(r.Context(), *queries.Address, queries.Amount)
		if err != nil {
			logger.Error(err, "unable to transfer")
			jsonhttp.InternalServerError(w, "unable to transfer amount")
			return
		}
		jsonhttp.OK(w, walletTxResponse{TransactionHash: txHash})
		return
	}

	nativeToken, err := s.chainBackend.BalanceAt(r.Context(), s.ethereumAddress, nil)
	if err != nil {
		logger.Error(err, "unable to acquire balance from the chain backend")
		jsonhttp.InternalServerError(w, "unable to acquire balance from the chain backend")
		return
	}

	if queries.Amount.Cmp(nativeToken) > 0 {
		jsonhttp.BadRequest(w, "not enough balance")
		return
	}

	req := &transaction.TxRequest{
		To:          queries.Address,
		GasPrice:    sctx.GetGasPrice(r.Context()),
		GasLimit:    sctx.GetGasLimitWithDefault(r.Context(), 300_000),
		Value:       queries.Amount,
		Description: "native token withdraw",
	}

	txHash, err := s.transaction.Send(r.Context(), req, transaction.DefaultTipBoostPercent)
	if err != nil {
		logger.Error(err, "unable to transfer")
		jsonhttp.InternalServerError(w, "unable to transfer")
		return
	}

	jsonhttp.OK(w, walletTxResponse{TransactionHash: txHash})
}

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
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

	txHash, err := s.walletWithdraw(r.Context(), *path.Coin, *queries.Address, queries.Amount)
	if err != nil {
		if strings.Contains(err.Error(), "not enough balance") {
			jsonhttp.BadRequest(w, err.Error())
			return
		}
		if strings.Contains(err.Error(), "only BZZ or NativeToken") {
			jsonhttp.BadRequest(w, err.Error())
			return
		}
		if strings.Contains(err.Error(), "provided address not whitelisted") {
			jsonhttp.BadRequest(w, err.Error())
			return
		}
		logger.Error(err, "withdraw failed")
		jsonhttp.InternalServerError(w, err.Error())
		return
	}

	jsonhttp.OK(w, walletTxResponse{TransactionHash: txHash})
}

func (s *Service) walletWithdraw(ctx context.Context, coin string, address common.Address, amount *big.Int) (common.Hash, error) {
	var bzz bool
	if strings.EqualFold("BZZ", coin) {
		bzz = true
	} else if !strings.EqualFold("NativeToken", coin) {
		return common.Hash{}, errors.New("only BZZ or NativeToken options are accepted")
	}

	if !slices.Contains(s.whitelistedWithdrawalAddress, address) {
		return common.Hash{}, errors.New("provided address not whitelisted")
	}

	if bzz {
		currentBalance, err := s.erc20Service.BalanceOf(ctx, s.ethereumAddress)
		if err != nil {
			return common.Hash{}, errors.New("unable to get balance")
		}

		if amount.Cmp(currentBalance) > 0 {
			return common.Hash{}, errors.New("not enough balance")
		}

		txHash, err := s.erc20Service.Transfer(ctx, address, amount)
		if err != nil {
			return common.Hash{}, errors.New("unable to transfer amount")
		}
		return txHash, nil
	}

	nativeToken, err := s.chainBackend.BalanceAt(ctx, s.ethereumAddress, nil)
	if err != nil {
		return common.Hash{}, errors.New("unable to acquire balance from the chain backend")
	}

	if amount.Cmp(nativeToken) > 0 {
		return common.Hash{}, errors.New("not enough balance")
	}

	req := &transaction.TxRequest{
		To:          &address,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    sctx.GetGasLimitWithDefault(ctx, 300_000),
		Value:       amount,
		Description: "native token withdraw",
	}

	txHash, err := s.transaction.Send(ctx, req, transaction.DefaultTipBoostPercent)
	if err != nil {
		return common.Hash{}, errors.New("unable to transfer")
	}

	return txHash, nil
}

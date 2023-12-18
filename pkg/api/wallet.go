// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/ethersphere/bee/pkg/log"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
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
		logger.Debug("unable to acquire balance from the chain backend", log.LogItem{"error", err})
		logger.Error(nil, "unable to acquire balance from the chain backend")
		jsonhttp.InternalServerError(w, "unable to acquire balance from the chain backend")
		return
	}

	bzz, err := s.erc20Service.BalanceOf(r.Context(), s.ethereumAddress)
	if err != nil {
		logger.Debug("unable to acquire erc20 balance", log.LogItem{"error", err})
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

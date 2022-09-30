// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type walletResponse struct {
	BZZ             *bigint.BigInt `json:"bzz"`             // the BZZ balance of the wallet associated with the eth address of the node
	XDai            *bigint.BigInt `json:"xDai"`            // the xDai balance of the wallet associated with the eth address of the node
	ChainID         int64          `json:"chainID"`         // the id of the block chain
	ContractAddress common.Address `json:"contractAddress"` // the address of the chequebook contract
}

func (s *Service) walletHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_wallet").Build()

	xdai, err := s.chainBackend.BalanceAt(r.Context(), s.ethereumAddress, nil)
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
		BZZ:             bigint.Wrap(bzz),
		XDai:            bigint.Wrap(xdai),
		ChainID:         s.chainID,
		ContractAddress: s.chequebook.Address(),
	})
}

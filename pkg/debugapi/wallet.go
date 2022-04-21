// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type walletResponse struct {
	BZZ             float64        `json:"bzz"`
	XDai            float64        `json:"xdai"`
	ChainID         int64          `json:"chainID"`
	ContractAddress common.Address `json:"contractAddress"`
}

func (s *Service) walletHandler(w http.ResponseWriter, r *http.Request) {

	xdaiInt, err := s.chainBackend.BalanceAt(r.Context(), s.ethereumAddress, nil)
	if err != nil {
		s.logger.Debugf("wallet: unable to acquire balance from chain backend: %v", err)
		s.logger.Debugf("wallet: unable to acquire balance from chain backend")
		jsonhttp.InternalServerError(w, "unable to acquire balance from chain backend")
		return
	}

	xdai, _ := new(big.Float).Quo(new(big.Float).SetInt(xdaiInt), big.NewFloat(1000000000000000000)).Float64()

	bzzInt, err := s.erc20Service.BalanceOf(r.Context(), s.ethereumAddress)
	if err != nil {
		s.logger.Debugf("wallet: unable to acquire erc20 balance: %v", err)
		s.logger.Debugf("wallet: unable to acquire erc20 balance")
		jsonhttp.InternalServerError(w, "unable to acquire erc20 balance")
		return
	}

	bzz, _ := new(big.Float).Quo(new(big.Float).SetInt(bzzInt), big.NewFloat(10000000000000000)).Float64()

	jsonhttp.OK(w, walletResponse{
		BZZ:             bzz,
		XDai:            xdai,
		ChainID:         s.chainID,
		ContractAddress: s.chequebook.Address(),
	})
}

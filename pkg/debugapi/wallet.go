// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
)

type walletResponse struct {
	BZZ             float64        `json:"bzz"`
	XDai            float64        `json:"xdai"`
	ChainID         int64          `json:"chainID"`
	ContractAddress common.Address `json:"contractAddress"`
}

func (s *Service) walletHandler(w http.ResponseWriter, r *http.Request) {

	xdai, err := s.chainBackend.BalanceAt(r.Context(), s.ethereumAddress, nil)
	if err != nil {
		s.logger.Debugf("wallet: unable to acquire balance from the chain backend: %v", err)
		s.logger.Errorf("wallet: unable to acquire balance from the chain backend")
		jsonhttp.InternalServerError(w, "unable to acquire balance from the chain backend")
		return
	}

	bzz, err := s.erc20Service.BalanceOf(r.Context(), s.ethereumAddress)
	if err != nil {
		s.logger.Debugf("wallet: unable to acquire erc20 balance: %v", err)
		s.logger.Errorf("wallet: unable to acquire erc20 balance")
		jsonhttp.InternalServerError(w, "unable to acquire erc20 balance")
		return
	}

	jsonhttp.OK(w, walletResponse{
		BZZ:             bigUnit(bzz, chequebook.Erc20SmallUnit),
		XDai:            bigUnit(xdai, chequebook.EthSmallUnit),
		ChainID:         s.chainID,
		ContractAddress: s.chequebook.Address(),
	})
}

func bigUnit(n *big.Int, subunit float64) float64 {
	f, _ := new(big.Float).Quo(new(big.Float).SetInt(n), big.NewFloat(subunit)).Float64()
	return f
}

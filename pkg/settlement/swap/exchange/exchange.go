// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exchange

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-price-oracle-abi/priceoracleabi"
)

var (
	rate      = big.NewInt(100000)
	deduction = big.NewInt(1000000)
)

type service struct {
	priceOracleAddress common.Address
	transactionService transaction.Service
}

type Service interface {
	// Deposit starts depositing erc20 token into the chequebook. This returns once the transactions has been broadcast.
	CurrentRates() (exchange *big.Int, deduce *big.Int)
}

var (
	priceOracleABI = transaction.ParseABIUnchecked(priceoracleabi.PriceOracleABIv0_1_0)
)

func New(priceOracleAddress common.Address, transactionService transaction.Service) Service {
	return &service{
		priceOracleAddress: priceOracleAddress,
		transactionService: transactionService,
	}
}

func (s *service) update(ctx context.Context) error {
	callData, err := priceOracleABI.Pack("getPrice")
	if err != nil {
		return err
	}
	result, err := s.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &s.priceOracleAddress,
		Data: callData,
	})
	if err != nil {
		return err
	}

	_ = result

	return nil
}

func (s *service) CurrentRates() (exchange *big.Int, deduce *big.Int) {
	return rate, deduction
}

// DiscoverPriceOracleAddress returns the canonical price oracle for this chainID
func DiscoverPriceOracleAddress(chainID int64) (priceOracleAddress common.Address, found bool) {
	if chainID == 5 {
		// goerli
		return common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2"), true
	}
	return common.Address{}, false
}

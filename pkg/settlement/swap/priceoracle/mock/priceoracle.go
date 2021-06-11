// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Service struct {
	rate   *big.Int
	deduct *big.Int
}

func New(rate *big.Int, deduct *big.Int) Service {
	return Service{
		rate:   rate,
		deduct: deduct,
	}
}

func (s Service) Start() {
}

func (s Service) GetPrice(ctx context.Context) (*big.Int, *big.Int, error) {
	return s.rate, s.deduct, nil
}

func (s Service) CurrentRates() (exchangeRate *big.Int, deduction *big.Int, err error) {
	return s.rate, s.deduct, nil
}

func (s Service) Close() error {
	return nil
}

func DiscoverPriceOracleAddress(chainID int64) (priceOracleAddress common.Address, found bool) {
	return common.Address{}, false
}

func (s Service) SetValues(rate *big.Int, deduct *big.Int) {
	s.rate = rate
	s.deduct = deduct
}

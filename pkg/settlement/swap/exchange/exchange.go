// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exchange

import (
	"math/big"
)

var (
	rate      = big.NewInt(100000)
	deduction = big.NewInt(1000000)
)

type service struct {
}

type Service interface {
	// Deposit starts depositing erc20 token into the chequebook. This returns once the transactions has been broadcast.
	CurrentRates() (exchange *big.Int, deduce *big.Int)
}

func New() (Service, error) {
	return &service{}, nil
}

func (s *service) CurrentRates() (exchange *big.Int, deduce *big.Int) {
	return rate, deduction
}

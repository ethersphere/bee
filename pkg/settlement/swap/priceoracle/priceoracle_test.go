// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priceoracle_test

import (
	"context"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/priceoracle"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/go-price-oracle-abi/priceoracleabi"
)

var priceOracleABI = abiutil.MustParseABI(priceoracleabi.PriceOracleABIv0_6_9)

func TestExchangeGetPrice(t *testing.T) {
	t.Parallel()

	priceOracleAddress := common.HexToAddress("0xabcd")

	expectedPrice := big.NewInt(100)
	expectedDeduce := big.NewInt(200)

	result := make([]byte, 64)
	expectedPrice.FillBytes(result[0:32])
	expectedDeduce.FillBytes(result[32:64])

	ex := priceoracle.New(
		log.Noop,
		priceOracleAddress,
		transactionmock.New(
			transactionmock.WithABICall(
				&priceOracleABI,
				priceOracleAddress,
				result,
				"getPrice",
			),
		),
		1,
	)

	price, deduce, err := ex.GetPrice(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if expectedPrice.Cmp(price) != 0 {
		t.Fatalf("got wrong price. wanted %d, got %d", expectedPrice, price)
	}

	if expectedDeduce.Cmp(deduce) != 0 {
		t.Fatalf("got wrong deduce. wanted %d, got %d", expectedDeduce, deduce)
	}
}

// TestCurrentRatesConcurrentWithUpdates is a regression test for RACE-01: the
// price-oracle poll loop writes exchangeRate and deduction while every
// settlement path reads them through CurrentRates. Run under -race, an
// unsynchronised write against the concurrent read is reported as fatal.
func TestCurrentRatesConcurrentWithUpdates(t *testing.T) {
	t.Parallel()

	ex := priceoracle.New(
		log.Noop,
		common.HexToAddress("0xabcd"),
		transactionmock.New(),
		1,
	)

	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 1; i <= iterations; i++ {
			priceoracle.SetRates(ex, big.NewInt(int64(i)), big.NewInt(int64(i)))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _, _ = ex.CurrentRates()
		}
	}()

	wg.Wait()
}

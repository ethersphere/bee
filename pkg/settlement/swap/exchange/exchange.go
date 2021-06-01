// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exchange

import (
	"context"
	"errors"
	"io"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-price-oracle-abi/priceoracleabi"
)

var (
	errDecodeABI          = errors.New("could not decode abi data")
	goerliContractAddress = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")
)

type service struct {
	logger             logging.Logger
	priceOracleAddress common.Address
	transactionService transaction.Service
	exchangeRate       *big.Int
	deduction          *big.Int
	timeDivisor        int64
	quitC              chan struct{}
}

type Service interface {
	io.Closer
	// Deposit starts depositing erc20 token into the chequebook. This returns once the transactions has been broadcast.
	CurrentRates() (exchange *big.Int, deduction *big.Int, err error)
	GetPrice(ctx context.Context) (*big.Int, *big.Int, error)
	Start()
}

var (
	priceOracleABI = transaction.ParseABIUnchecked(priceoracleabi.PriceOracleABIv0_1_0)
)

func New(logger logging.Logger, priceOracleAddress common.Address, transactionService transaction.Service, timeDivisor int64) Service {
	return &service{
		logger:             logger,
		priceOracleAddress: priceOracleAddress,
		transactionService: transactionService,
		exchangeRate:       big.NewInt(0),
		deduction:          nil,
		quitC:              make(chan struct{}),
		timeDivisor:        timeDivisor,
	}
}

func (s *service) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		<-s.quitC
	}()

	go func() {
		defer cancel()
		for {
			exchangeRate, deduction, err := s.getPrice(ctx)
			if err != nil {
				s.logger.Errorf("could not get price: %v", err)
			}

			s.exchangeRate = exchangeRate
			s.deduction = deduction

			ts := time.Now().Unix()

			timeUntilNextPoll := time.Duration(s.timeDivisor-ts%s.timeDivisor) * time.Second

			s.logger.Tracef("updated exchange rate to %d and deduction to %d", exchangeRate, deduction)

			select {
			case <-s.quitC:
				return
			case <-time.After(timeUntilNextPoll):
			}
		}
	}()
}

func (s *service) GetPrice(ctx context.Context) (*big.Int, *big.Int, error) {
	return s.getPrice(ctx)
}

func (s *service) getPrice(ctx context.Context) (*big.Int, *big.Int, error) {
	callData, err := priceOracleABI.Pack("getPrice")
	if err != nil {
		return nil, nil, err
	}
	result, err := s.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &s.priceOracleAddress,
		Data: callData,
	})
	if err != nil {
		return nil, nil, err
	}

	results, err := priceOracleABI.Unpack("getPrice", result)
	if err != nil {
		return nil, nil, err
	}

	if len(results) != 2 {
		return nil, nil, errDecodeABI
	}

	exchange, ok := abi.ConvertType(results[0], new(big.Int)).(*big.Int)
	if !ok || exchange == nil {
		return nil, nil, errDecodeABI
	}

	deduction, ok := abi.ConvertType(results[1], new(big.Int)).(*big.Int)
	if !ok || deduction == nil {
		return nil, nil, errDecodeABI
	}

	return exchange, deduction, nil
}

func (s *service) CurrentRates() (exchangeRate *big.Int, deduction *big.Int, err error) {
	if s.exchangeRate.Cmp(big.NewInt(0)) == 0 {
		return nil, nil, errors.New("exchange rate not yet available")
	}
	if s.deduction == nil {
		return nil, nil, errors.New("deduction amount not yet available")
	}
	return s.exchangeRate, s.deduction, nil
}

func (s *service) Close() error {
	close(s.quitC)
	return nil
}

// DiscoverPriceOracleAddress returns the canonical price oracle for this chainID
func DiscoverPriceOracleAddress(chainID int64) (priceOracleAddress common.Address, found bool) {
	if chainID == 5 {
		// goerli
		return goerliContractAddress, true
	}
	return common.Address{}, false
}

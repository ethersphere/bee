// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priceoracle

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
	errDecodeABI = errors.New("could not decode abi data")
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
	// CurrentRates returns the current value of exchange rate and deduction
	// according to the latest information from oracle
	CurrentRates() (exchangeRate *big.Int, deduction *big.Int, err error)
	// GetPrice retrieves latest available information from oracle
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
			exchangeRate, deduction, err := s.GetPrice(ctx)
			if err != nil {
				s.logger.Errorf("could not get price: %v", err)
			} else {
				s.logger.Tracef("updated exchange rate to %d and deduction to %d", exchangeRate, deduction)
				s.exchangeRate = exchangeRate
				s.deduction = deduction
			}

			ts := time.Now().Unix()

			// We poll the oracle in every timestamp divisible by constant 300 (timeDivisor)
			// in order to get latest version approximately at the same time on all nodes
			// and to minimize polling frequency
			// If the node gets newer information than what was applicable at last polling point at startup
			// this minimizes the negative scenario to less than 5 minutes
			// during which cheques can not be sent / accepted because of the asymmetric information
			timeUntilNextPoll := time.Duration(s.timeDivisor-ts%s.timeDivisor) * time.Second

			select {
			case <-s.quitC:
				return
			case <-time.After(timeUntilNextPoll):
			}
		}
	}()
}

func (s *service) GetPrice(ctx context.Context) (*big.Int, *big.Int, error) {
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

	exchangeRate, ok := abi.ConvertType(results[0], new(big.Int)).(*big.Int)
	if !ok || exchangeRate == nil {
		return nil, nil, errDecodeABI
	}

	deduction, ok := abi.ConvertType(results[1], new(big.Int)).(*big.Int)
	if !ok || deduction == nil {
		return nil, nil, errDecodeABI
	}

	return exchangeRate, deduction, nil
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

var (
	goerliChainID         = int64(5)
	goerliContractAddress = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")
	xdaiChainID           = int64(100)
	xdaiContractAddress   = common.HexToAddress("0x0FDc5429C50e2a39066D8A94F3e2D2476fcc3b85")
)

// DiscoverPriceOracleAddress returns the canonical price oracle for this chainID
func DiscoverPriceOracleAddress(chainID int64) (priceOracleAddress common.Address, found bool) {
	switch chainID {
	case goerliChainID:
		return goerliContractAddress, true
	case xdaiChainID:
		return xdaiContractAddress, true
	}
	return common.Address{}, false
}

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
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-price-oracle-abi/priceoracleabi"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "priceoracle"

var (
	errDecodeABI = errors.New("could not decode abi data")
)

type service struct {
	logger             log.Logger
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

func New(logger log.Logger, priceOracleAddress common.Address, transactionService transaction.Service, timeDivisor int64) Service {
	return &service{
		logger:             logger.WithName(loggerName).Register(),
		priceOracleAddress: priceOracleAddress,
		transactionService: transactionService,
		exchangeRate:       big.NewInt(0),
		deduction:          nil,
		quitC:              make(chan struct{}),
		timeDivisor:        timeDivisor,
	}
}

func (s *service) Start() {
	loggerV1 := s.logger.V(1).Register()

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
				s.logger.Error(err, "could not get price")
			} else {
				loggerV1.Debug("updated exchange rate and deduction", "new_exchange_rate", exchangeRate, "new_deduction", deduction)
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

func (s *service) CurrentRates() (exchangeRate, deduction *big.Int, err error) {
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

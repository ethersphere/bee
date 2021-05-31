// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	"errors"
	"math/big"

	"github.com/ethersphere/bee/pkg/p2p"
)

const (
	exchangeRateFieldName = "exchange"
	deductionFieldName    = "deduction"
)

var (
	// ErrFieldLength denotes p2p.Header having malformed field length in bytes
	ErrFieldLength = errors.New("field length error")
	// ErrNoExchangeHeader denotes p2p.Header lacking specified field
	ErrNoExchangeHeader = errors.New("no exchange header")
	// ErrNoTargetHeader denotes p2p.Header lacking specified field
	ErrNoDeductionHeader = errors.New("no deduction header")
)

func MakeSettlementHeaders(exchangeRate, deduction *big.Int) p2p.Headers {

	return p2p.Headers{
		exchangeRateFieldName: exchangeRate.Bytes(),
		deductionFieldName:    deduction.Bytes(),
	}
}

func ParseSettlementResponseHeaders(receivedHeaders p2p.Headers) (exchange *big.Int, deduction *big.Int, err error) {

	exchangeRate, err := ParseExchangeHeader(receivedHeaders)
	if err != nil {
		return nil, nil, err
	}

	deduction, err = ParseDeductionHeader(receivedHeaders)
	if err != nil {
		return exchangeRate, nil, err
	}

	return exchangeRate, deduction, nil
}

func ParseExchangeHeader(receivedHeaders p2p.Headers) (*big.Int, error) {
	if receivedHeaders[exchangeRateFieldName] == nil {
		return nil, ErrNoExchangeHeader
	}

	exchange := new(big.Int).SetBytes(receivedHeaders[exchangeRateFieldName])
	return exchange, nil
}

func ParseDeductionHeader(receivedHeaders p2p.Headers) (*big.Int, error) {
	if receivedHeaders[deductionFieldName] == nil {
		return nil, ErrNoDeductionHeader
	}

	deduced := new(big.Int).SetBytes(receivedHeaders[deductionFieldName])
	return deduced, nil
}

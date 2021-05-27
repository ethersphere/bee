// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	"encoding/binary"
	"errors"
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

func MakeSettlementHeaders(exchangeRate, deduction uint64) p2p.Headers {
	exchangeRateInBytes := make([]byte, 8)
	deductionInBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(exchangeRateInBytes, exchangeRate)
	binary.BigEndian.PutUint64(deductionInBytes, deduction)

	headers := p2p.Headers{
		exchangeRateFieldName: exchangeRateInBytes,
		deductionFieldName:    deductionInBytes,
	}

	return headers
}

func ParseSettlementResponseHeaders(receivedHeaders p2p.Headers) (uint64, uint64, error) {

	exchangeRate, err := ParseExchangeHeader(receivedHeaders)
	if err != nil {
		return 0, 0, err
	}

	deduction, err := ParseDeductionHeader(receivedHeaders)
	if err != nil {
		return 0, 0, err
	}

	return exchangeRate, deduction, nil
}

func ParseExchangeHeader(receivedHeaders p2p.Headers) (uint64, error) {
	if receivedHeaders[exchangeRateFieldName] == nil {
		return 0, ErrNoExchangeHeader
	}

	if len(receivedHeaders[exchangeRateFieldName]) != 8 {
		return 0, ErrFieldLength
	}

	exchange := binary.BigEndian.Uint64(receivedHeaders[exchangeRateFieldName])
	return exchange, nil
}

func ParseDeductionHeader(receivedHeaders p2p.Headers) (uint64, error) {
	if receivedHeaders[deductionFieldName] == nil {
		return 0, ErrNoDeductionHeader
	}

	if len(receivedHeaders[deductionFieldName]) != 8 {
		return 0, ErrFieldLength
	}

	deduced := binary.BigEndian.Uint64(receivedHeaders[deductionFieldName])
	return deduced, nil
}

// 0.00000000000000001 bzz
// 1 accounting

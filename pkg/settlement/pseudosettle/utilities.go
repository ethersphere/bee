// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	"encoding/binary"
	"errors"
	"github.com/ethersphere/bee/pkg/p2p"
	"math"
	"math/big"
)

const (
	allowanceFieldName = "allowance"
	timestampFieldName = "timestamp"
)

var (
	// ErrFieldLength denotes p2p.Header having malformed field length in bytes
	ErrFieldLength = errors.New("field length error")
	// ErrNoTimestampHeader denotes p2p.Header lacking specified field
	ErrNoTimestampHeader = errors.New("no timestamp header")
	// ErrNoAllowanceHeader denotes p2p.Header lacking specified field
	ErrNoAllowanceHeader = errors.New("no allowance header")
	// ErrTimestampValue denotes
	ErrTimestampValue = errors.New("timestamp out of bounds")
)

// Headers, utility functions

func MakeAllowanceResponseHeaders(limit *big.Int, stamp int64) (p2p.Headers, error) {

	timestampInBytes := make([]byte, 8)

	if stamp < 0 {
		return p2p.Headers{}, ErrTimestampValue
	}

	binary.BigEndian.PutUint64(timestampInBytes, uint64(stamp))

	limitInBytes := limit.Bytes()

	headers := p2p.Headers{
		allowanceFieldName: limitInBytes,
		timestampFieldName: timestampInBytes,
	}

	return headers, nil
}

func ParseAllowanceResponseHeaders(receivedHeaders p2p.Headers) (*big.Int, int64, error) {
	allowance, err := ParseAllowanceHeader(receivedHeaders)
	if err != nil {
		return big.NewInt(0), 0, err
	}
	timestamp, err := ParseTimestampHeader(receivedHeaders)
	if err != nil {
		return big.NewInt(0), 0, err
	}

	return allowance, timestamp, nil
}

func ParseAllowanceHeader(receivedHeaders p2p.Headers) (*big.Int, error) {

	if receivedHeaders[allowanceFieldName] == nil {
		return big.NewInt(0), ErrNoAllowanceHeader
	}

	receivedAllowance := big.NewInt(0).SetBytes(receivedHeaders[allowanceFieldName])

	return receivedAllowance, nil
}

func ParseTimestampHeader(receivedHeaders p2p.Headers) (int64, error) {
	if receivedHeaders[timestampFieldName] == nil {
		return 0, ErrNoTimestampHeader
	}
	if len(receivedHeaders[timestampFieldName]) != 8 {
		return 0, ErrFieldLength
	}

	receivedTimestamp := binary.BigEndian.Uint64(receivedHeaders[timestampFieldName])
	if receivedTimestamp > math.MaxInt64 {
		return 0, ErrTimestampValue
	}
	return int64(receivedTimestamp), nil
}

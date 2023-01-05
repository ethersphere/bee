// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abiutil

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

var (
	ErrTypecasting  = errors.New("typecasting failed")
	ErrEmptyResults = errors.New("empty results")
)

// MustParseABI parses is the same as calling abi.JSON
// but panics on error (if the given ABI is invalid).
func MustParseABI(json string) abi.ABI {
	val, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Errorf("unable to parse ABI: %w", err))
	}
	return val
}

func ConvertType(in interface{}, proto interface{}) (out interface{}, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("failed to convert type: %v", rec)
		}
	}()

	out = abi.ConvertType(in, proto)

	return
}

// ConvertBigInt converts a value to a big int in safe way by handling unintended failures in conversion.
func ConvertBigInt(values []interface{}) (*big.Int, error) {
	// values should have at least one value
	if len(values) == 0 {
		return nil, ErrEmptyResults
	}

	val, err := ConvertType(values[0], new(big.Int))
	if err != nil {
		return nil, fmt.Errorf("failed to convert type to big.Int: %w", err)
	}

	return val.(*big.Int), nil
}

// UnpackBool unpacks a value to a boolean in safe way by handling unintended failures in conversion.
func UnpackBool(values []interface{}) (bool, error) {
	// values should have at least one value
	if len(values) == 0 {
		return false, ErrEmptyResults
	}

	value, ok := values[0].(bool)
	if !ok {
		return false, ErrTypecasting
	}

	return value, nil
}

// UnpackBytes32 unpacks a value to a byte array in safe way by handling unintended failures in conversion.
func UnpackBytes32(values []interface{}) ([]byte, error) {
	// values should have at least one value
	if len(values) == 0 {
		return nil, ErrEmptyResults
	}
	value, ok := values[0].([32]byte)
	if !ok {
		return nil, ErrTypecasting
	}

	return value[:], nil
}

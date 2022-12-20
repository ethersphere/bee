// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package abiutil

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
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

func UnpackBigInt(d abi.ABI, result []byte, name string) (*big.Int, error) {
	values, err := d.Unpack(name, result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack %v name: %v", name, err)
	}

	// values should have at least one value
	if len(values) == 0 {
		return nil, fmt.Errorf("unexpected empty results")
	}

	val, err := ConvertType(values[0], new(big.Int))
	if err != nil {
		return nil, fmt.Errorf("failed to convert type to big.Int: %v", err)
	}

	return val.(*big.Int), nil
}

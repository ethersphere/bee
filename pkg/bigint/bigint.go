// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bigint

import (
	"encoding/json"
	"fmt"
	"math/big"
)

type BigInt struct {
	*big.Int
}

func (i *BigInt) MarshalJSON() ([]byte, error) {
	if i.Int == nil {
		return []byte("null"), nil
	}

	return fmt.Appendf(nil, `"%s"`, i.String()), nil
}

func (i *BigInt) UnmarshalJSON(b []byte) error {
	var val string
	err := json.Unmarshal(b, &val)
	if err != nil {
		return err
	}

	if i.Int == nil {
		i.Int = new(big.Int)
	}

	i.SetString(val, 10)

	return nil
}

// Wrap wraps big.Int pointer into BigInt struct.
func Wrap(i *big.Int) *BigInt {
	return &BigInt{Int: i}
}

// MarshalBinary serializes the BigInt quickly, prefixing a 1 for negatives, 0 for positives.
func (i *BigInt) MarshalBinary() ([]byte, error) {
	if i.Int == nil {
		return []byte{0}, nil
	}
	bytes := i.Bytes()
	res := make([]byte, len(bytes)+1)
	if i.Sign() < 0 {
		res[0] = 1
	}
	copy(res[1:], bytes)
	return res, nil
}

// UnmarshalBinary evaluates backward compatibility. If data begins with 0 or 1,
// it uses fast binary parsing. Otherwise, it delegates to json.Unmarshal.
func (i *BigInt) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	// Fallback to JSON if data doesn't start with binary signature
	if data[0] != 0 && data[0] != 1 {
		if i.Int == nil {
			i.Int = new(big.Int)
		}
		return json.Unmarshal(data, i.Int)
	}
	if i.Int == nil {
		i.Int = new(big.Int)
	}
	i.SetBytes(data[1:])
	if data[0] == 1 {
		i.Neg(i.Int)
	}
	return nil
}

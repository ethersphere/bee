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

// MarshalBinary implements encoding.BinaryMarshaler using Gob encoding.
// Panics if the underlying *big.Int is nil, as this indicates a programmer error.
func (i *BigInt) MarshalBinary() ([]byte, error) {
	if i.Int == nil {
		panic("bigint: MarshalBinary called on nil Int")
	}
	return i.GobEncode()
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler using Gob decoding.
func (i *BigInt) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("bigint: UnmarshalBinary called with empty data")
	}
	i.Int = new(big.Int)
	return i.GobDecode(data)
}

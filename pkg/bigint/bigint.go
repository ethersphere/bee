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
	if i.Int == nil {
		i.Int = new(big.Int)
	}

	var val string
	if err := json.Unmarshal(b, &val); err == nil {
		if _, ok := i.SetString(val, 10); !ok {
			return fmt.Errorf("bigint: invalid decimal string %q", val)
		}
		return nil
	}

	var num json.Number
	if err := json.Unmarshal(b, &num); err != nil {
		return err
	}
	if _, ok := i.SetString(num.String(), 10); !ok {
		return fmt.Errorf("bigint: invalid json number %q", num.String())
	}
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
	if err := i.GobDecode(data); err != nil {
		// fallback: try JSON
		return i.UnmarshalJSON(data)
	}
	return nil
}

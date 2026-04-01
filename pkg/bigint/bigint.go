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
func (i *BigInt) MarshalBinary() ([]byte, error) {
	if i.Int == nil {
		return []byte{}, nil
	}
	return i.GobEncode()
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler. It supports both
// Gob-encoded data (written by MarshalBinary) and legacy JSON-encoded data
// for backward compatibility with existing stored values.
// Gob-encoded big.Int always begins with byte 2 (positive) or 3 (negative).
// Legacy data stored via json.Marshal(*big.Int) is an unquoted decimal string.
// Legacy data stored via json.Marshal(bigint.BigInt) is a quoted decimal string.
func (i *BigInt) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if i.Int == nil {
		i.Int = new(big.Int)
	}
	if data[0] == 2 || data[0] == 3 {
		return i.GobDecode(data)
	}
	if data[0] == '"' {
		// quoted decimal string: e.g. "123" — from bigint.BigInt JSON marshaling
		return json.Unmarshal(data, i)
	}
	// unquoted decimal number: e.g. 123 or -456 — from json.Marshal(*big.Int)
	if _, ok := i.SetString(string(data), 10); !ok {
		return fmt.Errorf("bigint: cannot parse %q as decimal integer", data)
	}
	return nil
}

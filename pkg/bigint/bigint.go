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

	return []byte(fmt.Sprintf(`"%s"`, i.String())), nil
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

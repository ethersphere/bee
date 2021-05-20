// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import "math/big"

type ReserveState struct {
	Radius    uint8    `json:"radius"`
	Available int64    `json:"available"`
	Outer     *big.Int `json:"outer"` // lower value limit for outer layer = the further half of chunks
	Inner     *big.Int `json:"inner"`
}

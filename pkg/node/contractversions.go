// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
)

// applyAddressOverride returns a copy of vs with the address of the first
// version replaced by override when override is not empty. The name is used
// in the error message for a malformed override, e.g. "malformed <name> address".
func applyAddressOverride(vs []config.ContractVersion, override, name string) ([]config.ContractVersion, error) {
	if override == "" {
		return vs, nil
	}
	if !common.IsHexAddress(override) {
		return nil, errors.New("malformed " + name + " address")
	}
	out := make([]config.ContractVersion, len(vs))
	copy(out, vs)
	if len(out) == 0 {
		out = append(out, config.ContractVersion{})
	}
	out[0].Address = common.HexToAddress(override)
	return out, nil
}

// fromBlock converts a redistribution round to the first block of that round.
func fromBlock(round uint64) uint64 {
	return round * storageincentives.DefaultBlocksPerRound
}

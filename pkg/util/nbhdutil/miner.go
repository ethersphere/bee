// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nbhdutil

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func MineOverlay(ctx context.Context, p ecdsa.PublicKey, networkID uint64, targetNeighborhood string) (swarm.Address, []byte, error) {

	nonce := make([]byte, 32)

	neighborhood, err := swarm.ParseBitStrAddress(targetNeighborhood)
	if err != nil {
		return swarm.ZeroAddress, nil, err
	}
	prox := len(targetNeighborhood)

	i := uint64(0)
	for {

		select {
		case <-ctx.Done():
			return swarm.ZeroAddress, nil, ctx.Err()
		default:
		}

		binary.LittleEndian.PutUint64(nonce, i)

		swarmAddress, err := crypto.NewOverlayAddress(p, networkID, nonce)
		if err != nil {
			return swarm.ZeroAddress, nil, fmt.Errorf("compute overlay address: %w", err)
		}

		if swarm.Proximity(swarmAddress.Bytes(), neighborhood.Bytes()) >= uint8(prox) {
			return swarmAddress, nonce, nil
		}

		i++
	}
}

// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package neighborhood

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

var errBadCharacter = errors.New("bad character in binary address")

func MineOverlay(ctx context.Context, p ecdsa.PublicKey, networkID uint64, neighborhoodPrefix string) (swarm.Address, []byte, error) {

	nonce := make([]byte, 32)

	neighborHood, err := bitStrToAddress(neighborhoodPrefix)
	if err != nil {
		return swarm.ZeroAddress, nil, err
	}
	prox := len(neighborhoodPrefix)

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

		if swarm.Proximity(swarmAddress.Bytes(), neighborHood.Bytes()) >= uint8(prox) {
			return swarmAddress, nonce, nil
		}

		i++
	}
}

func bitStrToAddress(src string) (swarm.Address, error) {

	bitPos := 7
	b := uint8(0)

	var a []byte

	for _, s := range src {
		if s == '1' {
			b |= 1 << bitPos
		} else if s != '0' {
			return swarm.ZeroAddress, errBadCharacter
		}
		bitPos--
		if bitPos < 0 {
			a = append(a, b)
			b = 0
			bitPos = 7
		}
	}

	a = append(a, b)

	return bytesToAddr(a), nil
}

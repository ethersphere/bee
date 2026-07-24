// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"math/rand"

	"github.com/ethersphere/bee/v2/pkg/cac"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// randAddr returns a random swarm address. It is a testing.TB-free port of
// swarm.RandAddress so it can be used from non-test code.
func randAddr(rng *rand.Rand) swarm.Address {
	b := make([]byte, swarm.HashSize)
	_, _ = rng.Read(b)
	return swarm.NewAddress(b)
}

// randAddrAt returns a random address at proximity order prox relative to base.
// It is a testing.TB-free port of swarm.RandAddressAt.
func randAddrAt(rng *rand.Rand, base swarm.Address, prox int) swarm.Address {
	addr := make([]byte, len(base.Bytes()))
	copy(addr, base.Bytes())

	pos := -1
	if prox >= 0 {
		pos = prox / 8
		trans := prox % 8
		var transbytea byte
		for j := 0; j <= trans; j++ {
			transbytea |= 1 << uint8(7-j)
		}
		flipbyte := byte(1 << uint8(7-trans))
		transbyteb := transbytea ^ byte(255)
		randbyte := byte(rng.Intn(255))
		addr[pos] = ((addr[pos] & transbytea) ^ flipbyte) | randbyte&transbyteb
	}

	for i := pos + 1; i < len(addr); i++ {
		addr[i] = byte(rng.Intn(255))
	}

	return swarm.NewAddress(addr)
}

// chunkAt mines a random-payload content-addressed chunk whose address is at
// proximity order >= minPO from target, and attaches a random (accept-all)
// postage stamp. The client only checks cac.Valid and the accept-all stamp fn,
// so the stamp need not be cryptographically valid.
func chunkAt(rng *rand.Rand, target swarm.Address, minPO uint8) swarm.Chunk {
	data := make([]byte, swarm.ChunkSize)
	for {
		_, _ = rng.Read(data)
		ch, err := cac.New(data)
		if err != nil {
			continue
		}
		if swarm.Proximity(ch.Address().Bytes(), target.Bytes()) >= minPO {
			return ch.WithStamp(postagetesting.MustNewStamp())
		}
	}
}

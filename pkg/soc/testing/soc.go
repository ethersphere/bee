// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// MockSoc defines a mocked soc with exported fields for easy testing.
type MockSoc struct {
	ID           soc.ID
	Owner        []byte
	Signature    []byte
	WrappedChunk swarm.Chunk
}

// Address returns the soc address of the mocked soc.
func (ms MockSoc) Address() swarm.Address {
	addr, _ := soc.CreateAddress(ms.ID, ms.Owner)
	return addr
}

// Chunk returns the soc chunk of the mocked soc.
func (ms MockSoc) Chunk() swarm.Chunk {
	return swarm.NewChunk(ms.Address(), append(ms.ID, append(ms.Signature, ms.WrappedChunk.Data()...)...))
}

// GenerateMockSoc generates a valid mocked soc from given data.
// If data is nil it generates random data.
func GenerateMockSoc(t *testing.T, data []byte) *MockSoc {
	t.Helper()

	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	if data == nil {
		data = make([]byte, swarm.ChunkSize)
		_, err = rand.Read(data)
		if err != nil {
			t.Fatal(err)
		}
	}
	ch, err := cac.New(data)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, soc.IdSize)
	hasher := swarm.NewHasher()
	_, err = hasher.Write(append(id, ch.Address().Bytes()...))
	if err != nil {
		t.Fatal(err)
	}

	signature, err := signer.Sign(hasher.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}

	return &MockSoc{
		ID:           id,
		Owner:        owner.Bytes(),
		Signature:    signature,
		WrappedChunk: ch,
	}
}

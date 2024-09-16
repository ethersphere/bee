// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// MockSOC defines a mocked SOC with exported fields for easy testing.
type MockSOC struct {
	ID           soc.ID
	Owner        []byte
	Signature    []byte
	WrappedChunk swarm.Chunk
}

// Address returns the SOC address of the mocked SOC.
func (ms MockSOC) Address() swarm.Address {
	addr, _ := soc.CreateAddress(ms.ID, ms.Owner)
	return addr
}

// Chunk returns the SOC chunk of the mocked SOC.
func (ms MockSOC) Chunk() swarm.Chunk {
	return swarm.NewChunk(ms.Address(), append(ms.ID, append(ms.Signature, ms.WrappedChunk.Data()...)...))
}

// GenerateMockSocWithSigner generates a valid mocked SOC from given data and signer.
func GenerateMockSocWithSigner(t *testing.T, data []byte, signer crypto.Signer) *MockSOC {
	t.Helper()

	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	ch, err := cac.New(data)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, swarm.HashSize)
	hasher := swarm.NewHasher()
	_, err = hasher.Write(append(id, ch.Address().Bytes()...))
	if err != nil {
		t.Fatal(err)
	}

	signature, err := signer.Sign(hasher.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}

	return &MockSOC{
		ID:           id,
		Owner:        owner.Bytes(),
		Signature:    signature,
		WrappedChunk: ch,
	}
}

// GenerateMockSOC generates a valid mocked SOC from given data.
func GenerateMockSOC(t *testing.T, data []byte) *MockSOC {
	t.Helper()

	ch, err := cac.New(data)
	if err != nil {
		t.Fatal(err)
	}

	return generateMockSOC(t, ch)
}

// GenerateMockSOC generates a valid mocked SOC from given chunk data (span + payload).
func GenerateMockSOCWithSpan(t *testing.T, data []byte) *MockSOC {
	t.Helper()

	ch, err := cac.NewWithDataSpan(data)
	if err != nil {
		t.Fatal(err)
	}

	return generateMockSOC(t, ch)
}

func generateMockSOC(t *testing.T, ch swarm.Chunk) *MockSOC {
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

	id := make([]byte, swarm.HashSize)
	hasher := swarm.NewHasher()
	_, err = hasher.Write(append(id, ch.Address().Bytes()...))
	if err != nil {
		t.Fatal(err)
	}

	signature, err := signer.Sign(hasher.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}

	return &MockSOC{
		ID:           id,
		Owner:        owner.Bytes(),
		Signature:    signature,
		WrappedChunk: ch,
	}
}

// GenerateMockSOCWithKey generates a valid mocked SOC from given data and key.
func GenerateMockSOCWithKey(t *testing.T, data []byte, privKey *ecdsa.PrivateKey) *MockSOC {
	t.Helper()

	signer := crypto.NewDefaultSigner(privKey)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cac.New(data)
	if err != nil {
		t.Fatal(err)
	}

	id := make([]byte, swarm.HashSize)
	hasher := swarm.NewHasher()
	_, err = hasher.Write(append(id, ch.Address().Bytes()...))
	if err != nil {
		t.Fatal(err)
	}

	signature, err := signer.Sign(hasher.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}

	return &MockSOC{
		ID:           id,
		Owner:        owner.Bytes(),
		Signature:    signature,
		WrappedChunk: ch,
	}
}

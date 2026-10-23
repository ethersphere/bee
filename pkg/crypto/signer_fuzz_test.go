// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/crypto/eip712"
)

// FuzzRecover fuzzes crypto.Recover, which recovers a public key from an
// attacker-supplied signature over attacker-supplied data. Both byte slices
// are fuzzed independently. The target exercises the 65-byte length guard and
// the subsequent make([]byte,65)/btcsig[0]=signature[64]/copy slicing plus the
// hashWithEthereumPrefix(data) step before btcecdsa.RecoverCompact. It asserts
// that no (signature, data) pair can cause a panic, and that a nil error always
// yields a non-nil public key.
func FuzzRecover(f *testing.F) {
	data := []byte("hello swarm")
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	sig, err := crypto.NewDefaultSigner(key).Sign(data)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(sig, data)
	f.Add(make([]byte, 65), []byte{})
	f.Add([]byte{}, []byte{})
	f.Add(make([]byte, 64), []byte("x"))

	f.Fuzz(func(t *testing.T, sig, data []byte) {
		pbk, err := crypto.Recover(sig, data)
		if err != nil {
			return
		}
		if pbk == nil {
			t.Fatal("nil error but nil public key")
		}
	})
}

var fuzzTypedData = &eip712.TypedData{
	Domain: eip712.TypedDataDomain{
		Name:    "test",
		Version: "1.0",
	},
	Types: eip712.Types{
		"EIP712Domain": {
			{
				Name: "name",
				Type: "string",
			},
			{
				Name: "version",
				Type: "string",
			},
		},
		"MyType": {
			{
				Name: "test",
				Type: "string",
			},
		},
	},
	Message: eip712.TypedDataMessage{
		"test": "abc",
	},
	PrimaryType: "MyType",
}

// FuzzRecoverEIP712 fuzzes crypto.RecoverEIP712 against a fixed valid
// *eip712.TypedData. Only the signature bytes are attacker-controlled here, so
// the target exercises the same 65-byte length guard and btcsig slicing as
// FuzzRecover. It asserts no signature input panics and that a nil error yields
// a non-nil public key.
func FuzzRecoverEIP712(f *testing.F) {
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	sig, err := crypto.NewDefaultSigner(key).SignTypedData(fuzzTypedData)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(sig)
	f.Add(make([]byte, 65))
	f.Add([]byte{})
	f.Add(make([]byte, 64))

	f.Fuzz(func(t *testing.T, sig []byte) {
		pbk, err := crypto.RecoverEIP712(sig, fuzzTypedData)
		if err != nil {
			return
		}
		if pbk == nil {
			t.Fatal("nil error but nil public key")
		}
	})
}

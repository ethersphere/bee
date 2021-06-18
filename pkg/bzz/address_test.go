// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"

	ma "github.com/multiformats/go-multiaddr"
)

func TestBzzAddress(t *testing.T) {
	node1ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	trxHash := common.HexToHash("0x1").Bytes()
	blockHash := common.HexToHash("0x2").Bytes()

	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err := crypto.NewOverlayAddress(privateKey1.PublicKey, 3, blockHash)
	if err != nil {
		t.Fatal(err)
	}
	signer1 := crypto.NewDefaultSigner(privateKey1)

	bzzAddress, err := bzz.NewAddress(signer1, node1ma, overlay, 3, trxHash)
	if err != nil {
		t.Fatal(err)
	}

	bzzAddress2, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), bzzAddress.Signature, trxHash, blockHash, 3)
	if err != nil {
		t.Fatal(err)
	}

	if !bzzAddress.Equal(bzzAddress2) {
		t.Fatalf("got %s expected %s", bzzAddress2, bzzAddress)
	}

	bytes, err := bzzAddress.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var newbzz bzz.Address
	if err := newbzz.UnmarshalJSON(bytes); err != nil {
		t.Fatal(err)
	}

	if !newbzz.Equal(bzzAddress) {
		t.Fatalf("got %s expected %s", newbzz, bzzAddress)
	}
}

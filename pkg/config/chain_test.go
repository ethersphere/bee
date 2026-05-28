// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"context"
	"encoding/hex"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// TestDeriveChequebookBytecodeHash derives the keccak256(eth_getCode) hash for
// a known-good chequebook deployed by a factory. Run this manually after a
// factory upgrade to get the new hash for AcceptedChequebookBytecodeHashes.
//
// Usage:
//
//	RPC_URL=https://... CHEQUEBOOK_ADDR=0x... go test ./pkg/config/... -run TestDeriveChequebookBytecodeHash -v
func TestDeriveChequebookBytecodeHash(t *testing.T) {
	rpcURL := os.Getenv("RPC_URL")
	addrStr := os.Getenv("CHEQUEBOOK_ADDR")
	if rpcURL == "" || addrStr == "" {
		t.Skip("set RPC_URL and CHEQUEBOOK_ADDR to run")
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		t.Fatalf("dial %s: %v", rpcURL, err)
	}
	defer client.Close()

	addr := common.HexToAddress(addrStr)
	code, err := client.CodeAt(context.Background(), addr, nil)
	if err != nil {
		t.Fatalf("CodeAt %s: %v", addr, err)
	}
	if len(code) == 0 {
		t.Fatalf("no deployed code at %s", addr)
	}

	hash := crypto.Keccak256Hash(code)
	t.Logf("add to AcceptedChequebookBytecodeHashes in pkg/config/chain.go:")
	t.Logf("\tmustHash(%q)", hex.EncodeToString(hash[:]))
}

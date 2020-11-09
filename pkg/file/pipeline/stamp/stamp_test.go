// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stamp_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/mock"
	"github.com/ethersphere/bee/pkg/file/pipeline/stamp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var id = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

func TestStampWriter(t *testing.T) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	owner, err := crypto.NewEthereumAddress(privKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	var (
		signer          = crypto.NewDefaultSigner(privKey)
		mockChainWriter = mock.NewChainWriter()
		st              = postage.NewStampIssuer("label", "keyID", id, 16, 8)
		stamper         = postage.NewStamper(st, signer)
		writer          = stamp.NewStampWriter(stamper, mockChainWriter)
	)

	args := pipeline.PipeWriteArgs{Ref: []byte{1, 2, 3, 4}}
	err = writer.ChainWrite(&args)
	if err != nil {
		t.Fatal(err)
	}
	if err := args.Stamp.Valid(swarm.NewAddress(args.Ref), owner); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(args.Stamp.BatchID(), id) {
		t.Fatalf("want batch id %v got %v", id, args.Stamp.BatchID())
	}

	if calls := mockChainWriter.ChainWriteCalls(); calls != 1 {
		t.Errorf("wanted 1 ChainWrite call, got %d", calls)
	}
}

// TestSum tests that calling Sum on the store writer results in Sum on the next writer in the chain.
func TestSum(t *testing.T) {
	mockChainWriter := mock.NewChainWriter()
	writer := stamp.NewStampWriter(nil, mockChainWriter)
	_, err := writer.Sum()
	if err != nil {
		t.Fatal(err)
	}
	if calls := mockChainWriter.SumCalls(); calls != 1 {
		t.Fatalf("wanted 1 Sum call but got %d", calls)
	}
}

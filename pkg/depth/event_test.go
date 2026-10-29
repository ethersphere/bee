// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/depth"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	redistabi "github.com/ethersphere/go-storage-incentives-abi/abi"
)

func truthLog(t *testing.T, a abi.ABI, hash [32]byte, d uint8, block uint64) types.Log {
	t.Helper()

	ev := a.Events["TruthSelected"]
	data, err := ev.Inputs.NonIndexed().Pack(hash, d)
	if err != nil {
		t.Fatal(err)
	}
	return types.Log{
		BlockNumber: block,
		Topics:      []common.Hash{ev.ID},
		Data:        data,
	}
}

func TestDecodeTruthSelected(t *testing.T) {
	t.Parallel()

	a := abiutil.MustParseABI(redistabi.MainnetRedistributionABI)
	l := truthLog(t, a, [32]byte{1, 2, 3}, 10, 4242)
	l.TxIndex = 3
	l.Index = 7

	got, err := depth.DecodeTruthSelected(&a, a.Events["TruthSelected"].ID, l)
	if err != nil {
		t.Fatal(err)
	}
	want := depth.Sample{Block: 4242, TxIndex: 3, LogIndex: 7, Depth: 10}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDecodeTruthSelectedWrongTopic(t *testing.T) {
	t.Parallel()

	a := abiutil.MustParseABI(redistabi.MainnetRedistributionABI)
	l := truthLog(t, a, [32]byte{1}, 10, 1)
	l.Topics[0] = a.Events["CountReveals"].ID

	if _, err := depth.DecodeTruthSelected(&a, a.Events["TruthSelected"].ID, l); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeTruthSelectedNoTopics(t *testing.T) {
	t.Parallel()

	a := abiutil.MustParseABI(redistabi.MainnetRedistributionABI)
	l := truthLog(t, a, [32]byte{1}, 10, 1)
	l.Topics = nil

	if _, err := depth.DecodeTruthSelected(&a, a.Events["TruthSelected"].ID, l); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeTruthSelectedShortData(t *testing.T) {
	t.Parallel()

	a := abiutil.MustParseABI(redistabi.MainnetRedistributionABI)
	l := truthLog(t, a, [32]byte{1}, 10, 1)
	l.Data = l.Data[:40]

	if _, err := depth.DecodeTruthSelected(&a, a.Events["TruthSelected"].ID, l); err == nil {
		t.Fatal("expected error")
	}
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package depth_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/depth"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	redistabi "github.com/ethersphere/go-storage-incentives-abi/abi"
)

func FuzzDecodeTruthSelected(f *testing.F) {
	a := abiutil.MustParseABI(redistabi.MainnetRedistributionABI)
	ev := a.Events["TruthSelected"]

	// Valid seed built the production way.
	valid, err := ev.Inputs.NonIndexed().Pack([32]byte{1, 2, 3}, uint8(10))
	if err != nil {
		f.Fatalf("pack seed: %v", err)
	}
	f.Add(valid, false)

	// Benign edge seeds.
	f.Add([]byte{}, false)
	f.Add(valid[:40], false)
	f.Add(append(append([]byte{}, valid...), 0), false)
	f.Add(make([]byte, 1024), false)
	f.Add(valid, true)

	f.Fuzz(func(t *testing.T, data []byte, wrongTopic bool) {
		topic := ev.ID
		if wrongTopic {
			topic = a.Events["CountReveals"].ID
		}
		l := types.Log{Topics: []common.Hash{topic}, Data: data}

		// Invariant (1): never panics on arbitrary bytes.
		s, err := depth.DecodeTruthSelected(&a, ev.ID, l)
		if err != nil {
			return
		}

		// Invariant (2): acceptance implies the right topic and a 64-byte payload.
		if wrongTopic {
			t.Fatal("accepted a log with the wrong topic")
		}
		if len(data) != 64 {
			t.Fatalf("accepted data of length %d", len(data))
		}
		if s.Depth != data[63] {
			t.Fatalf("depth %d does not match data[63] %d", s.Depth, data[63])
		}
	})
}

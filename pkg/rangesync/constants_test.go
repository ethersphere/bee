// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"math/rand/v2"
	"runtime"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestConstants(t *testing.T) {
	t.Parallel()

	if rangesync.StartupGuardTimeout != 2*rangesync.StatusMaxInterval {
		t.Errorf("StartupGuardTimeout %s is not 2 x StatusMaxInterval %s", rangesync.StartupGuardTimeout, rangesync.StatusMaxInterval)
	}
	if rangesync.SyncRateWindow != puller.DefaultHistRateWindow {
		t.Errorf("SyncRateWindow %s differs from the puller's %s", rangesync.SyncRateWindow, puller.DefaultHistRateWindow)
	}
	if rangesync.LeafDepthSlack != fingerprint.FanoutBits {
		t.Errorf("LeafDepthSlack %d is not one split level (%d)", rangesync.LeafDepthSlack, fingerprint.FanoutBits)
	}
	if rangesync.StatusMinInterval >= rangesync.StatusMaxInterval {
		t.Errorf("StatusMinInterval %s not below StatusMaxInterval %s", rangesync.StatusMinInterval, rangesync.StatusMaxInterval)
	}
	if rangesync.MinRoundInterval >= rangesync.MaxRoundInterval {
		t.Errorf("MinRoundInterval %s not below MaxRoundInterval %s", rangesync.MinRoundInterval, rangesync.MaxRoundInterval)
	}
	if rangesync.MaxMessageBytes != 1<<20 {
		t.Errorf("MaxMessageBytes: got %d, want 1 MiB", rangesync.MaxMessageBytes)
	}
}

// TestSessionDecoderMemory: a decoder that received a session's worth of
// symbols keeps at most codedSymbolBytes per received symbol and
// DecoderBytesPerID per recovered ID (SessionDecoderMemory). It measures the
// heap the decoder holds, so it does not run in parallel.
func TestSessionDecoderMemory(t *testing.T) {
	m := fingerprint.NewFloatMapper()
	r := rand.New(rand.NewPCG(1, 2))
	ids := make([]fingerprint.ID, rangesync.SessionSymbolBudget*10/16) // decodes within the budget
	for i := range ids {
		for j := range ids[i] {
			ids[i][j] = byte(r.Uint32())
		}
	}
	remote := make([]fingerprint.CodedSymbol, rangesync.SessionSymbolBudget)
	local := make([]fingerprint.CodedSymbol, len(remote))
	for _, id := range ids {
		fingerprint.Encode(m, remote, 0, id, 1)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	dec := fingerprint.NewDecoder(m, fingerprint.NewRange(swarm.ZeroAddress, 0))
	const batch = 64
	for start := 0; start < len(remote) && !dec.Done(); start += batch {
		end := min(start+batch, len(remote))
		if err := dec.Add(uint64(start), remote[start:end], local[start:end]); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	if !dec.Done() || dec.Recovered() != len(ids) {
		t.Fatalf("decoded %d of %d (done %v)", dec.Recovered(), len(ids), dec.Done())
	}
	const codedSymbolBytes = 88 // fingerprint.CodedSymbol
	held := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	symbols := int64(dec.Received()) * codedSymbolBytes
	limit := symbols + int64(dec.Recovered())*rangesync.DecoderBytesPerID
	t.Logf("received %d, recovered %d: %d B held, %.0f B per recovered ID beyond the symbols", dec.Received(), dec.Recovered(), held, float64(held-symbols)/float64(dec.Recovered()))
	if held > limit {
		t.Fatalf("decoder holds %d B, over the budget of %d B", held, limit)
	}
	runtime.KeepAlive(dec)
}

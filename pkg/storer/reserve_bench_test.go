// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// BenchmarkReservePut measures the throughput of ReservePutter().Put on a disk
// storer with stamped chunks spread over the first bins and a reserve large
// enough that nothing is evicted. It is the baseline the per-element cost of
// the rangesync fingerprint update is compared with (rangesync plan T1.2,
// measured with -benchtime=20000x: each run generates b.N chunks up front).
func BenchmarkReservePut(b *testing.B) {
	baseAddr := swarm.RandAddress(b)
	st, err := diskStorer(b, dbTestOps(baseAddr, 1<<22, nil, nil, time.Hour))()
	if err != nil {
		b.Fatal(err)
	}

	batches := []*postage.Batch{postagetesting.MustNewBatch(), postagetesting.MustNewBatch(), postagetesting.MustNewBatch()}
	chunks := make([]swarm.Chunk, b.N)
	for i := range chunks {
		ch := chunk.GenerateTestRandomChunkAt(b, baseAddr, i%8)
		chunks[i] = ch.WithStamp(postagetesting.MustNewBatchStamp(batches[i%len(batches)].ID))
	}

	ctx := context.Background()
	putter := st.ReservePutter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := putter.Put(ctx, chunks[i]); err != nil {
			b.Fatal(err)
		}
	}
}

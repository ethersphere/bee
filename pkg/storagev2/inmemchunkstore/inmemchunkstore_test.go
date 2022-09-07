// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore_test

import (
	"testing"

	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
)

func TestChunkStore(t *testing.T) {
	storagetest.TestChunkStore(t, inmem.New())
}

func BenchmarkChunkStore(t *testing.B) {
	storagetest.RunChunkStoreBenchmarkTests(t, inmem.New())
}

/* Benchmark results
go1.17.2
goos: linux
goarch: amd64
pkg: github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore
cpu: Intel(R) Core(TM) i7-7820HQ CPU @ 2.90GHz
BenchmarkChunkStore
BenchmarkChunkStore/WriteSequential
BenchmarkChunkStore/WriteSequential-8         	 1308024	       863.9 ns/op	     272 B/op	       6 allocs/op
BenchmarkChunkStore/WriteRandom
BenchmarkChunkStore/WriteRandom-8             	 1857764	      1031 ns/op	     293 B/op	       6 allocs/op
BenchmarkChunkStore/ReadSequential
BenchmarkChunkStore/ReadSequential-8          	 5851861	       560.7 ns/op	      32 B/op	       2 allocs/op
BenchmarkChunkStore/ReadRandom
BenchmarkChunkStore/ReadRandom-8              	 3309765	       374.7 ns/op	      32 B/op	       2 allocs/op
BenchmarkChunkStore/ReadRandomMissing
BenchmarkChunkStore/ReadRandomMissing-8       	 4385307	       231.1 ns/op	      32 B/op	       2 allocs/op
BenchmarkChunkStore/ReadReverse
BenchmarkChunkStore/ReadReverse-8             	 5706280	       376.4 ns/op	      32 B/op	       2 allocs/op
BenchmarkChunkStore/ReadRedHot
BenchmarkChunkStore/ReadRedHot-8              	 6876939	       320.3 ns/op	      32 B/op	       2 allocs/op
BenchmarkChunkStore/IterateSequential
BenchmarkChunkStore/IterateSequential-8       	 3109309	       363.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkChunkStore/IterateReverse
    /home/anatol/swarm/bee/pkg/storagev2/inmemchunkstore/chunkstore.go:245: not implemented
--- SKIP: BenchmarkChunkStore/IterateReverse
BenchmarkChunkStore/DeleteRandom
BenchmarkChunkStore/DeleteRandom-8            	 1716336	       624.8 ns/op	     116 B/op	       3 allocs/op
BenchmarkChunkStore/DeleteSequential
BenchmarkChunkStore/DeleteSequential-8        	 3003429	       492.7 ns/op	     114 B/op	       3 allocs/op
PASS
coverage: 75.0% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore	141.284s
*/

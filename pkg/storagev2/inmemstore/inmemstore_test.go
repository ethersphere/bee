// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore_test

import (
	"testing"

	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
)

func TestStore(t *testing.T) {
	storagetest.TestStore(t, inmem.New())
}

func BenchmarkStore(b *testing.B) {
	st := inmem.New()
	storagetest.RunStoreBenchmarkTests(b, st)
}

/* Benchmark results
go: 1.17.12
goos: linux
goarch: amd64
pkg: github.com/ethersphere/bee/pkg/storagev2/inmemstore
cpu: Intel(R) Core(TM) i7-7820HQ CPU @ 2.90GHz
BenchmarkInMem
BenchmarkInMem/WriteSequential
BenchmarkInMem/WriteSequential-8         	 1858598	       607.9 ns/op	     364 B/op	       7 allocs/op
BenchmarkInMem/WriteRandom
BenchmarkInMem/WriteRandom/parallelism-1
BenchmarkInMem/WriteRandom/parallelism-1-8         	 1000000	      1130 ns/op	     312 B/op	       6 allocs/op
BenchmarkInMem/WriteRandom/parallelism-2
BenchmarkInMem/WriteRandom/parallelism-2-8         	  728137	      2300 ns/op	     624 B/op	      12 allocs/op
BenchmarkInMem/WriteRandom/parallelism-4
BenchmarkInMem/WriteRandom/parallelism-4-8         	  344992	      5316 ns/op	    1248 B/op	      24 allocs/op
BenchmarkInMem/WriteRandom/parallelism-8
BenchmarkInMem/WriteRandom/parallelism-8-8         	  133194	     11385 ns/op	    2496 B/op	      48 allocs/op
BenchmarkInMem/ReadSequential
BenchmarkInMem/ReadSequential-8                    	 1490608	       854.4 ns/op	     152 B/op	       5 allocs/op
BenchmarkInMem/ReadRandom
BenchmarkInMem/ReadRandom-8                        	  973734	      2089 ns/op	     152 B/op	       5 allocs/op
BenchmarkInMem/ReadRandomMissing
BenchmarkInMem/ReadRandomMissing-8                 	 1000000	      1118 ns/op	      96 B/op	       3 allocs/op
BenchmarkInMem/ReadReverse
BenchmarkInMem/ReadReverse-8                       	 1587060	       915.5 ns/op	     152 B/op	       5 allocs/op
BenchmarkInMem/ReadRedHot
BenchmarkInMem/ReadRedHot-8                        	 1934049	       877.2 ns/op	     152 B/op	       5 allocs/op
BenchmarkInMem/IterateSequential
BenchmarkInMem/IterateSequential-8                 	  612537	      1683 ns/op	     573 B/op	      12 allocs/op
BenchmarkInMem/IterateReverse
BenchmarkInMem/IterateReverse-8                    	       1	1311065959 ns/op	695364040 B/op	 7757261 allocs/op
BenchmarkInMem/DeleteRandom:_1
BenchmarkInMem/DeleteRandom:_1-8                   	 1326961	      1206 ns/op	      96 B/op	       3 allocs/op
BenchmarkInMem/DeleteSequential:_1
BenchmarkInMem/DeleteSequential:_1-8               	 3694153	       348.1 ns/op	      96 B/op	       3 allocs/op
PASS
coverage: 61.2% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/inmemstore	73.190s
*/

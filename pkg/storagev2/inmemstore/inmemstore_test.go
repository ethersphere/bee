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
BenchmarkStore
BenchmarkStore/WriteSequential
BenchmarkStore/WriteSequential-8         	 2021545	       624.2 ns/op	     353 B/op	       7 allocs/op
BenchmarkStore/WriteRandom
BenchmarkStore/WriteRandom/parallelism-1
BenchmarkStore/WriteRandom/parallelism-1-8         	 1000000	      1125 ns/op	     304 B/op	       6 allocs/op
BenchmarkStore/WriteRandom/parallelism-2
BenchmarkStore/WriteRandom/parallelism-2-8         	  841941	      2352 ns/op	     608 B/op	      12 allocs/op
BenchmarkStore/WriteRandom/parallelism-4
BenchmarkStore/WriteRandom/parallelism-4-8         	  373220	      4681 ns/op	    1216 B/op	      24 allocs/op
BenchmarkStore/WriteRandom/parallelism-8
BenchmarkStore/WriteRandom/parallelism-8-8         	  151621	      9566 ns/op	    2432 B/op	      48 allocs/op
BenchmarkStore/WriteRandom/parallelism-16
BenchmarkStore/WriteRandom/parallelism-16-8        	   67851	     21628 ns/op	    4864 B/op	      96 allocs/op
BenchmarkStore/WriteRandom/parallelism-32
BenchmarkStore/WriteRandom/parallelism-32-8        	   29742	     37412 ns/op	    9729 B/op	     192 allocs/op
BenchmarkStore/WriteRandom/parallelism-64
BenchmarkStore/WriteRandom/parallelism-64-8        	   14744	     73593 ns/op	   19457 B/op	     384 allocs/op
BenchmarkStore/WriteRandom/parallelism-128
BenchmarkStore/WriteRandom/parallelism-128-8       	    9981	    166797 ns/op	   38914 B/op	     768 allocs/op
BenchmarkStore/WriteRandom/parallelism-256
BenchmarkStore/WriteRandom/parallelism-256-8       	    5008	    323746 ns/op	   77834 B/op	    1536 allocs/op
BenchmarkStore/WriteRandom/parallelism-512
BenchmarkStore/WriteRandom/parallelism-512-8       	    2576	    607722 ns/op	  155681 B/op	    3072 allocs/op
BenchmarkStore/WriteRandom/parallelism-1024
BenchmarkStore/WriteRandom/parallelism-1024-8      	    1198	   1149086 ns/op	  311453 B/op	    6146 allocs/op
BenchmarkStore/WriteRandom/parallelism-2048
BenchmarkStore/WriteRandom/parallelism-2048-8      	     530	   2416686 ns/op	  623227 B/op	   12298 allocs/op
BenchmarkStore/ReadSequential
BenchmarkStore/ReadSequential-8                    	 1346355	       824.5 ns/op	     144 B/op	       5 allocs/op
BenchmarkStore/ReadRandom
BenchmarkStore/ReadRandom-8                        	  865430	      1430 ns/op	     144 B/op	       5 allocs/op
BenchmarkStore/ReadRandomMissing
BenchmarkStore/ReadRandomMissing-8                 	 7073697	       194.1 ns/op	      88 B/op	       3 allocs/op
BenchmarkStore/ReadReverse
BenchmarkStore/ReadReverse-8                       	 1472763	       838.0 ns/op	     144 B/op	       5 allocs/op
BenchmarkStore/ReadRedHot
BenchmarkStore/ReadRedHot-8                        	 1487623	       881.6 ns/op	     144 B/op	       5 allocs/op
BenchmarkStore/IterateSequential
BenchmarkStore/IterateSequential-8                 	 2328874	       566.9 ns/op	     152 B/op	       4 allocs/op
BenchmarkStore/IterateReverse
BenchmarkStore/IterateReverse-8                    	       1	2111586998 ns/op	846223912 B/op	 9315543 allocs/op
BenchmarkStore/DeleteRandom
BenchmarkStore/DeleteRandom-8                      	 1000000	      1171 ns/op	      88 B/op	       3 allocs/op
BenchmarkStore/DeleteSequential
BenchmarkStore/DeleteSequential-8                  	 3072211	       391.8 ns/op	      88 B/op	       3 allocs/op
PASS
coverage: 61.2% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/inmemstore	102.678s
*/

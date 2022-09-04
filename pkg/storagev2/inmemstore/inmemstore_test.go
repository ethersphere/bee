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

func BenchmarkInMem(b *testing.B) {
	st := inmem.New()
	storetesting.RunBenchmarkTests(b, st)
}

/* Benchmark results
go: 1.17.12
goos: linux
goarch: amd64
pkg: github.com/ethersphere/bee/pkg/storagev2/inmemstore
cpu: Intel(R) Core(TM) i7-7820HQ CPU @ 2.90GHz
BenchmarkInMem
BenchmarkInMem/WriteSequential
BenchmarkInMem/WriteSequential-8         	 1943894	       592.6 ns/op	     348 B/op	       6 allocs/op
BenchmarkInMem/WriteRandom
BenchmarkInMem/WriteRandom/parallelism-1
BenchmarkInMem/WriteRandom/parallelism-1-8         	 1000000	      1104 ns/op	     312 B/op	       6 allocs/op
BenchmarkInMem/WriteRandom/parallelism-2
BenchmarkInMem/WriteRandom/parallelism-2-8         	  813448	      2335 ns/op	     624 B/op	      12 allocs/op
BenchmarkInMem/WriteRandom/parallelism-4
BenchmarkInMem/WriteRandom/parallelism-4-8         	  306218	      4742 ns/op	    1248 B/op	      24 allocs/op
BenchmarkInMem/WriteRandom/parallelism-8
BenchmarkInMem/WriteRandom/parallelism-8-8         	  144076	      9020 ns/op	    2496 B/op	      48 allocs/op
BenchmarkInMem/ReadSequential
BenchmarkInMem/ReadSequential-8                    	 1914870	       659.5 ns/op	     152 B/op	       5 allocs/op
BenchmarkInMem/ReadRandom
BenchmarkInMem/ReadRandom-8                        	 1000000	      1223 ns/op	     152 B/op	       5 allocs/op
BenchmarkInMem/ReadRandomMissing
BenchmarkInMem/ReadRandomMissing-8                 	 1423952	       875.0 ns/op	      96 B/op	       3 allocs/op
BenchmarkInMem/ReadReverse
BenchmarkInMem/ReadReverse-8                       	 1736295	       851.1 ns/op	     152 B/op	       5 allocs/op
BenchmarkInMem/IterateSequential
BenchmarkInMem/IterateSequential-8                 	  628188	      1789 ns/op	     573 B/op	      12 allocs/op
BenchmarkInMem/IterateReverse
BenchmarkInMem/IterateReverse-8                    	       1	1762817715 ns/op	696860480 B/op	 7796641 allocs/op
BenchmarkInMem/DeleteRandom:_1
BenchmarkInMem/DeleteRandom:_1-8                   	 1291303	       888.9 ns/op	      96 B/op	       3 allocs/op
BenchmarkInMem/DeleteSequential:_1
BenchmarkInMem/DeleteSequential:_1-8               	 3618207	       343.8 ns/op	      96 B/op	       3 allocs/op
PASS
coverage: 61.2% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/inmemstore	60.552s
*/

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
)

func TestStoreTestSuite(t *testing.T) {
	store, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	storagetest.TestStore(t, store)
}

func BenchmarkLevelDB(b *testing.B) {
	st, err := ldb.New("", new(opt.Options))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storetesting.RunBenchmarkTests(b, st)
}

/* Benchmark results
go: 1.17.12
goos: linux
goarch: amd64
pkg: github.com/ethersphere/bee/pkg/storagev2/leveldb
cpu: Intel(R) Core(TM) i7-7820HQ CPU @ 2.90GHz
BenchmarkLevelDB
BenchmarkLevelDB/WriteSequential
BenchmarkLevelDB/WriteSequential-8         	  681466	      1520 ns/op	    1041 B/op	       8 allocs/op
BenchmarkLevelDB/WriteRandom
BenchmarkLevelDB/WriteRandom/parallelism-1
BenchmarkLevelDB/WriteRandom/parallelism-1-8         	  675507	      4390 ns/op	    2932 B/op	       9 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-2
BenchmarkLevelDB/WriteRandom/parallelism-2-8         	  196368	      7741 ns/op	    4609 B/op	      17 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-4
BenchmarkLevelDB/WriteRandom/parallelism-4-8         	   96027	     12658 ns/op	    5927 B/op	      31 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-8
BenchmarkLevelDB/WriteRandom/parallelism-8-8         	   46778	     24776 ns/op	   10757 B/op	      58 allocs/op
BenchmarkLevelDB/ReadSequential
BenchmarkLevelDB/ReadSequential-8                    	 1000000	      3663 ns/op	    1827 B/op	      20 allocs/op
BenchmarkLevelDB/ReadRandom
BenchmarkLevelDB/ReadRandom-8                        	  389064	      5524 ns/op	    1248 B/op	      23 allocs/op
BenchmarkLevelDB/ReadRandomMissing
BenchmarkLevelDB/ReadRandomMissing-8                 	  377690	      5380 ns/op	    1188 B/op	      21 allocs/op
BenchmarkLevelDB/ReadReverse
BenchmarkLevelDB/ReadReverse-8                       	  978600	      3758 ns/op	    1842 B/op	      21 allocs/op
BenchmarkLevelDB/ReadRedHot
BenchmarkLevelDB/ReadRedHot-8                        	 1000000	      3488 ns/op	    1802 B/op	      19 allocs/op
BenchmarkLevelDB/IterateSequential
BenchmarkLevelDB/IterateSequential-8                 	  406318	      4393 ns/op	    3087 B/op	      15 allocs/op
BenchmarkLevelDB/IterateReverse
BenchmarkLevelDB/IterateReverse-8                    	  405747	      3697 ns/op	    2705 B/op	      15 allocs/op
BenchmarkLevelDB/DeleteRandom:_1
BenchmarkLevelDB/DeleteRandom:_1-8                   	  819007	      2179 ns/op	     978 B/op	       6 allocs/op
BenchmarkLevelDB/DeleteSequential:_1
BenchmarkLevelDB/DeleteSequential:_1-8               	  860095	      1305 ns/op	     455 B/op	       6 allocs/op
PASS
coverage: 60.6% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/leveldb	54.031s
*/

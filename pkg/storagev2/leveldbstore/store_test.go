// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
	ldb "github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestStoreTestSuite(t *testing.T) {
	store, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	storagetest.TestStore(t, store)
}

func BenchmarkStore(b *testing.B) {
	st, err := ldb.New("", &opt.Options{
		Compression: opt.SnappyCompression,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storagetest.RunStoreBenchmarkTests(b, st)
}

/* Benchmark results
go: 1.17.12
goos: linux
goarch: amd64
pkg: github.com/ethersphere/bee/pkg/storagev2/leveldbstore
cpu: Intel(R) Core(TM) i7-7820HQ CPU @ 2.90GHz
BenchmarkStore
BenchmarkStore/WriteSequential
BenchmarkStore/WriteSequential-8         	  605460	      1745 ns/op	    1034 B/op	       8 allocs/op
BenchmarkStore/WriteRandom
BenchmarkStore/WriteRandom/parallelism-1
BenchmarkStore/WriteRandom/parallelism-1-8         	  629793	      4096 ns/op	    2671 B/op	       9 allocs/op
BenchmarkStore/WriteRandom/parallelism-2
BenchmarkStore/WriteRandom/parallelism-2-8         	  188066	      8314 ns/op	    4602 B/op	      17 allocs/op
BenchmarkStore/WriteRandom/parallelism-4
BenchmarkStore/WriteRandom/parallelism-4-8         	   75294	     13460 ns/op	    5764 B/op	      31 allocs/op
BenchmarkStore/WriteRandom/parallelism-8
BenchmarkStore/WriteRandom/parallelism-8-8         	   41826	     29072 ns/op	    9655 B/op	      57 allocs/op
BenchmarkStore/WriteRandom/parallelism-16
BenchmarkStore/WriteRandom/parallelism-16-8        	   19225	     62657 ns/op	   18191 B/op	     100 allocs/op
BenchmarkStore/WriteRandom/parallelism-32
BenchmarkStore/WriteRandom/parallelism-32-8        	   10000	    120598 ns/op	   34850 B/op	     187 allocs/op
BenchmarkStore/WriteRandom/parallelism-64
BenchmarkStore/WriteRandom/parallelism-64-8        	    4750	    260106 ns/op	   71143 B/op	     354 allocs/op
BenchmarkStore/WriteRandom/parallelism-128
BenchmarkStore/WriteRandom/parallelism-128-8       	    2029	    570119 ns/op	  112630 B/op	     677 allocs/op
BenchmarkStore/WriteRandom/parallelism-256
BenchmarkStore/WriteRandom/parallelism-256-8       	     987	   1221940 ns/op	  259537 B/op	    1348 allocs/op
BenchmarkStore/WriteRandom/parallelism-512
BenchmarkStore/WriteRandom/parallelism-512-8       	     482	   2562216 ns/op	  594146 B/op	    2685 allocs/op
BenchmarkStore/WriteRandom/parallelism-1024
BenchmarkStore/WriteRandom/parallelism-1024-8      	     232	   5027585 ns/op	  865239 B/op	    5422 allocs/op
BenchmarkStore/WriteRandom/parallelism-2048
BenchmarkStore/WriteRandom/parallelism-2048-8      	     114	  10367100 ns/op	 1669523 B/op	   11012 allocs/op
BenchmarkStore/ReadSequential
BenchmarkStore/ReadSequential-8                    	  849603	      4066 ns/op	    1805 B/op	      21 allocs/op
BenchmarkStore/ReadRandom
BenchmarkStore/ReadRandom-8                        	  335145	      6650 ns/op	    1244 B/op	      24 allocs/op
BenchmarkStore/ReadRandomMissing
BenchmarkStore/ReadRandomMissing-8                 	 1414250	       836.9 ns/op	     208 B/op	       7 allocs/op
BenchmarkStore/ReadReverse
BenchmarkStore/ReadReverse-8                       	  581599	      3798 ns/op	    1459 B/op	      20 allocs/op
BenchmarkStore/ReadRedHot
BenchmarkStore/ReadRedHot-8                        	  518310	      4617 ns/op	    1794 B/op	      20 allocs/op
BenchmarkStore/IterateSequential
BenchmarkStore/IterateSequential-8                 	 1727948	       758.8 ns/op	     511 B/op	       4 allocs/op
BenchmarkStore/IterateReverse
BenchmarkStore/IterateReverse-8                    	 1488330	       809.9 ns/op	     512 B/op	       5 allocs/op
BenchmarkStore/DeleteRandom
BenchmarkStore/DeleteRandom-8                      	  736826	      2246 ns/op	     844 B/op	       6 allocs/op
BenchmarkStore/DeleteSequential
BenchmarkStore/DeleteSequential-8                  	  928838	      1296 ns/op	     626 B/op	       6 allocs/op
PASS
coverage: 59.8% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/leveldbstore	96.105s
*/

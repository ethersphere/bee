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
BenchmarkLevelDB/WriteSequential-8         	  740870	      1674 ns/op	    1043 B/op	       8 allocs/op
BenchmarkLevelDB/WriteRandom
BenchmarkLevelDB/WriteRandom/parallelism-1
BenchmarkLevelDB/WriteRandom/parallelism-1-8         	  746833	      4561 ns/op	    3165 B/op	       9 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-2
BenchmarkLevelDB/WriteRandom/parallelism-2-8         	  223671	      7636 ns/op	    4745 B/op	      17 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-4
BenchmarkLevelDB/WriteRandom/parallelism-4-8         	  102396	     12449 ns/op	    6230 B/op	      31 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-8
BenchmarkLevelDB/WriteRandom/parallelism-8-8         	   46778	     25570 ns/op	   10652 B/op	      58 allocs/op
BenchmarkLevelDB/ReadSequential
BenchmarkLevelDB/ReadSequential-8                    	  305744	      3307 ns/op	    1634 B/op	      19 allocs/op
BenchmarkLevelDB/ReadRandom
BenchmarkLevelDB/ReadRandom-8                        	  392686	      5487 ns/op	    1222 B/op	      23 allocs/op
BenchmarkLevelDB/ReadRandomMissing
BenchmarkLevelDB/ReadRandomMissing-8                 	  398671	      5391 ns/op	    1194 B/op	      21 allocs/op
BenchmarkLevelDB/ReadReverse
BenchmarkLevelDB/ReadReverse-8                       	  768434	      3521 ns/op	    1826 B/op	      21 allocs/op
BenchmarkLevelDB/IterateSequential
BenchmarkLevelDB/IterateSequential-8                 	  370545	      4061 ns/op	    2611 B/op	      15 allocs/op
BenchmarkLevelDB/IterateReverse
BenchmarkLevelDB/IterateReverse-8                    	  377454	      3768 ns/op	    2747 B/op	      15 allocs/op
BenchmarkLevelDB/DeleteRandom:_1
BenchmarkLevelDB/DeleteRandom:_1-8                   	  682662	      1979 ns/op	     911 B/op	       6 allocs/op
BenchmarkLevelDB/DeleteSequential:_1
BenchmarkLevelDB/DeleteSequential:_1-8               	  820357	      1274 ns/op	     555 B/op	       6 allocs/op
PASS
coverage: 60.6% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/leveldb	32.747s
*/

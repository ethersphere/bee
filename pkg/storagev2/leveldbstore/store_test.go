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
	st, err := ldb.New("", new(opt.Options))
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
pkg: github.com/ethersphere/bee/pkg/storagev2/leveldb
cpu: Intel(R) Core(TM) i7-7820HQ CPU @ 2.90GHz
BenchmarkLevelDB
BenchmarkLevelDB/WriteSequential
BenchmarkLevelDB/WriteSequential-8         	  567930	      1906 ns/op	    1052 B/op	       8 allocs/op
BenchmarkLevelDB/WriteRandom
BenchmarkLevelDB/WriteRandom/parallelism-1
BenchmarkLevelDB/WriteRandom/parallelism-1-8         	  615745	      4280 ns/op	    2763 B/op	       9 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-2
BenchmarkLevelDB/WriteRandom/parallelism-2-8         	  218419	      7572 ns/op	    4465 B/op	      17 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-4
BenchmarkLevelDB/WriteRandom/parallelism-4-8         	   97946	     16256 ns/op	    5949 B/op	      31 allocs/op
BenchmarkLevelDB/WriteRandom/parallelism-8
BenchmarkLevelDB/WriteRandom/parallelism-8-8         	   35602	     49921 ns/op	   10198 B/op	      54 allocs/op
BenchmarkLevelDB/ReadSequential
BenchmarkLevelDB/ReadSequential-8                    	  369727	      3738 ns/op	    1677 B/op	      19 allocs/op
BenchmarkLevelDB/ReadRandom
BenchmarkLevelDB/ReadRandom-8                        	  298050	      5633 ns/op	    1190 B/op	      22 allocs/op
BenchmarkLevelDB/ReadRandomMissing
BenchmarkLevelDB/ReadRandomMissing-8                 	  355465	      5907 ns/op	    1187 B/op	      21 allocs/op
BenchmarkLevelDB/ReadReverse
BenchmarkLevelDB/ReadReverse-8                       	  596318	      4226 ns/op	    1796 B/op	      20 allocs/op
BenchmarkLevelDB/ReadRedHot
BenchmarkLevelDB/ReadRedHot-8                        	  920134	      3479 ns/op	    1809 B/op	      20 allocs/op
BenchmarkLevelDB/IterateSequential
BenchmarkLevelDB/IterateSequential-8                 	  366224	      4735 ns/op	    2782 B/op	      15 allocs/op
BenchmarkLevelDB/IterateReverse
BenchmarkLevelDB/IterateReverse-8                    	  343334	      3825 ns/op	    2713 B/op	      15 allocs/op
BenchmarkLevelDB/DeleteRandom:_1
BenchmarkLevelDB/DeleteRandom:_1-8                   	  850855	      1953 ns/op	     833 B/op	       6 allocs/op
BenchmarkLevelDB/DeleteSequential:_1
BenchmarkLevelDB/DeleteSequential:_1-8               	  872286	      1312 ns/op	     509 B/op	       6 allocs/op
PASS
coverage: 60.6% of statements
ok  	github.com/ethersphere/bee/pkg/storagev2/leveldb	47.395s
*/

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"unsafe"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

// Bucket symbol nodes of 16 and 32 symbols (BucketSymbols, proposal §5.3),
// as Group adapters of the Fenwick tree the Index keeps over its buckets.
type (
	bucketSyms16 [16]fingerprint.CodedSymbol
	bucketSyms32 [32]fingerprint.CodedSymbol
)

func (b *bucketSyms16) Add(o *bucketSyms16) {
	for i := range b {
		b[i].Add(&o[i])
	}
}

func (b *bucketSyms16) Sub(o *bucketSyms16) {
	for i := range b {
		b[i].Sub(&o[i])
	}
}
func (b *bucketSyms16) Zero() { *b = bucketSyms16{} }

func (b *bucketSyms32) Add(o *bucketSyms32) {
	for i := range b {
		b[i].Add(&o[i])
	}
}

func (b *bucketSyms32) Sub(o *bucketSyms32) {
	for i := range b {
		b[i].Sub(&o[i])
	}
}
func (b *bucketSyms32) Zero() { *b = bucketSyms32{} }

// benchBucketTree measures the bucket symbol tree at B = 4096 buckets with
// nodes of len(T) symbols: an element update (what Apply and Commit do per
// element, through the node's symbol indices below len(T)), the worst bucket
// run read (23 nodes, the Symbols path of a bucket run) and the tree's
// memory.
func benchBucketTree[T any, P interface {
	*T
	fingerprint.Group[T]
}](b *testing.B, syms func(*T) []fingerprint.CodedSymbol) {
	b.Helper()
	const buckets = 4096
	f := fingerprint.NewFenwick[T, P](buckets)
	m := fingerprint.NewFloatMapper()
	r := rand.New(rand.NewPCG(1, 2))
	ids := make([]fingerprint.ID, 1024)
	for i := range ids {
		ids[i] = randomIDIn(r, fingerprint.Range{})
	}
	var zero T
	b.Run("update", func(b *testing.B) {
		i := 0
		for b.Loop() {
			id := ids[i%len(ids)]
			f.Update(i%buckets, func(node *T) {
				fingerprint.Encode(m, syms(node), 0, id, 1)
			})
			i++
		}
		b.ReportMetric(float64(buckets*unsafe.Sizeof(zero))/(1<<20), "MiB/tree")
	})
	b.Run("range/worst", func(b *testing.B) {
		var out T
		for b.Loop() {
			f.Range(2047, 4095, &out)
		}
	})
}

// BenchmarkBucketSymbols compares BucketSymbols 16 and 32 (proposal §5.3:
// memory against one more range descent for bucket runs with more than
// ~11 differences). The decode sub-benchmarks report, for d differences in
// a bucket run, the fraction of streams that decode within 16 and within 32
// symbols, and the symbols needed at the median.
func BenchmarkBucketSymbols(b *testing.B) {
	b.Run("16", func(b *testing.B) {
		benchBucketTree[bucketSyms16](b, func(s *bucketSyms16) []fingerprint.CodedSymbol { return s[:] })
	})
	b.Run("32", func(b *testing.B) {
		benchBucketTree[bucketSyms32](b, func(s *bucketSyms32) []fingerprint.CodedSymbol { return s[:] })
	})
	m := fingerprint.NewFloatMapper()
	for _, d := range []int{4, 8, 11, 16, 20, 24} {
		b.Run(fmt.Sprintf("decode/d=%d", d), func(b *testing.B) {
			r := rand.New(rand.NewPCG(uint64(d), 3))
			var in16, in32 int
			var needs []int
			for b.Loop() { // one stream per iteration
				k := symbolsToDecode(b, m, r, d, 64)
				needs = append(needs, k)
				if k <= 16 {
					in16++
				}
				if k <= 32 {
					in32++
				}
			}
			b.ReportMetric(float64(in16)/float64(b.N), "p_decode16")
			b.ReportMetric(float64(in32)/float64(b.N), "p_decode32")
			b.ReportMetric(float64(median(needs)), "median_symbols")
		})
	}
}

// symbolsToDecode returns the number of symbols after which a stream of d
// random differences (split between the two sides) decodes, or limit+1.
func symbolsToDecode(tb testing.TB, m fingerprint.Mapper, r *rand.Rand, d, limit int) int {
	tb.Helper()
	remote := make([]fingerprint.CodedSymbol, limit)
	local := make([]fingerprint.CodedSymbol, limit)
	for i := range d {
		if i%2 == 0 {
			fingerprint.Encode(m, remote, 0, randomIDIn(r, fingerprint.Range{}), 1)
		} else {
			fingerprint.Encode(m, local, 0, randomIDIn(r, fingerprint.Range{}), 1)
		}
	}
	dec := fingerprint.NewDecoder(m, fingerprint.Range{})
	for k := range limit {
		if err := dec.Add(uint64(k), remote[k:k+1], local[k:k+1]); err != nil {
			tb.Fatal(err)
		}
		if dec.Done() {
			return k + 1
		}
	}
	return limit + 1
}

func median(xs []int) int {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[len(s)/2]
}

// BenchmarkIndexApplyPopulated measures Apply on an index of 1<<18
// elements at the protocol LeafBits, for a single put (one add) and a put
// with an eviction (one add, one remove), the deltas of a reserve Put. Its
// ops/s compare with the reserve's put throughput.
func BenchmarkIndexApplyPopulated(b *testing.B) {
	r := rand.New(rand.NewPCG(9, 10))
	ix := newIndex(0)
	if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(symbolsIDs(r, 1<<18))); err != nil {
		b.Fatal(err)
	}
	adds := symbolsIDs(r, 4096)
	b.Run("add", func(b *testing.B) {
		i := 0
		for b.Loop() {
			ix.Apply(adds[i%len(adds):i%len(adds)+1], nil)
			i++
		}
	})
	b.Run("add+evict", func(b *testing.B) {
		i := 0
		for b.Loop() {
			j := i % len(adds)
			ix.Apply(adds[j:j+1], adds[(j+1)%len(adds):(j+1)%len(adds)+1])
			i++
		}
	})
}

// BenchmarkIndexRebuildScale measures Rebuild at reserve scales up to the
// full reserve (4.2M elements, 2^22). Select a size with
// -bench 'IndexRebuildScale/n=4194304' -benchtime 1x; the IDs alone take
// 128 MiB at 2^21 elements.
func BenchmarkIndexRebuildScale(b *testing.B) {
	for _, n := range []int{1 << 16, 1 << 18, 1 << 20, 4194304} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			ids := symbolsIDs(rand.New(rand.NewPCG(5, 6)), n)
			ix := newIndex(0)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := ix.Rebuild(fingerprint.Range{}, sliceIterate(ids)); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/element")
			b.ReportMetric(fingerprint.IndexMemoryBytes(ix)/(1<<20), "index_MiB")
		})
	}
}

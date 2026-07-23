// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	test "github.com/ethersphere/bee/v2/pkg/file/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestPartialWrites(t *testing.T) {
	t.Parallel()

	m := inmemchunkstore.New()
	p := builder.NewPipelineBuilder(context.Background(), m, false, 0)
	_, _ = p.Write([]byte("hello "))
	_, _ = p.Write([]byte("world"))

	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exp := swarm.MustParseHexAddress("92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f")
	if !bytes.Equal(exp.Bytes(), sum) {
		t.Fatalf("expected %s got %s", exp.String(), hex.EncodeToString(sum))
	}
}

func TestHelloWorld(t *testing.T) {
	t.Parallel()

	m := inmemchunkstore.New()
	p := builder.NewPipelineBuilder(context.Background(), m, false, 0)

	data := []byte("hello world")
	_, err := p.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exp := swarm.MustParseHexAddress("92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f")
	if !bytes.Equal(exp.Bytes(), sum) {
		t.Fatalf("expected %s got %s", exp.String(), hex.EncodeToString(sum))
	}
}

// TestEmpty tests that a hash is generated for an empty file.
func TestEmpty(t *testing.T) {
	t.Parallel()

	m := inmemchunkstore.New()
	p := builder.NewPipelineBuilder(context.Background(), m, false, 0)

	data := []byte{}
	_, err := p.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exp := swarm.MustParseHexAddress("b34ca8c22b9e982354f9c7f50b470d66db428d880c8a904d5fe4ec9713171526")
	if !bytes.Equal(exp.Bytes(), sum) {
		t.Fatalf("expected %s got %s", exp.String(), hex.EncodeToString(sum))
	}
}

func TestAllVectors(t *testing.T) {
	t.Parallel()

	for i := 1; i <= 20; i++ {
		data, expect := test.GetVector(t, i)
		t.Run(fmt.Sprintf("data length %d, vector %d", len(data), i), func(t *testing.T) {
			t.Parallel()

			m := inmemchunkstore.New()
			p := builder.NewPipelineBuilder(context.Background(), m, false, 0)

			_, err := p.Write(data)
			if err != nil {
				t.Fatal(err)
			}
			sum, err := p.Sum()
			if err != nil {
				t.Fatal(err)
			}
			a := swarm.NewAddress(sum)
			if !a.Equal(expect) {
				t.Fatalf("failed run %d, expected address %s but got %s", i, expect.String(), a.String())
			}
		})
	}
}

/*
go test -v -bench=. -run Bench -benchmem
goos: linux
goarch: amd64
pkg: github.com/ethersphere/bee/pkg/file/pipeline/builder
BenchmarkPipeline
BenchmarkPipeline/1000-bytes
BenchmarkPipeline/1000-bytes-4         	   14475	     75170 ns/op	   63611 B/op	     333 allocs/op
BenchmarkPipeline/10000-bytes
BenchmarkPipeline/10000-bytes-4        	    2775	    459275 ns/op	  321825 B/op	    1826 allocs/op
BenchmarkPipeline/100000-bytes
BenchmarkPipeline/100000-bytes-4       	     334	   3523558 ns/op	 1891672 B/op	   11994 allocs/op
BenchmarkPipeline/1000000-bytes
BenchmarkPipeline/1000000-bytes-4      	      36	  33140883 ns/op	17745116 B/op	  114170 allocs/op
BenchmarkPipeline/10000000-bytes
BenchmarkPipeline/10000000-bytes-4     	       4	 304759595 ns/op	175378648 B/op	 1135082 allocs/op
BenchmarkPipeline/100000000-bytes
BenchmarkPipeline/100000000-bytes-4    	       1	3064439098 ns/op	1751509528 B/op	11342736 allocs/op
PASS
ok  	github.com/ethersphere/bee/pkg/file/pipeline/builder	17.599s
*/
func BenchmarkPipeline(b *testing.B) {
	for _, count := range []int{
		1000,      // 1k
		10000,     // 10 k
		100000,    // 100 k
		1000000,   // 1 mb
		10000000,  // 10 mb
		100000000, // 100 mb
	} {
		b.Run(strconv.Itoa(count)+"-bytes", func(b *testing.B) {
			for b.Loop() {
				benchmarkPipeline(b, count)
			}
		})
	}
}

func benchmarkPipeline(b *testing.B, count int) {
	b.Helper()

	b.StopTimer()

	data := testutil.RandBytes(b, count)
	m := inmemchunkstore.New()
	p := builder.NewPipelineBuilder(context.Background(), m, false, 0)

	b.StartTimer()

	_, err := p.Write(data)
	if err != nil {
		b.Fatal(err)
	}
	_, err = p.Sum()
	if err != nil {
		b.Fatal(err)
	}
}

// copyingChunkStore copies chunk data on Put. inmemchunkstore retains the
// caller's slice, so copying here isolates the pipeline's aliasing from the
// store's own.
type copyingChunkStore struct {
	storage.ChunkStore
}

func (c *copyingChunkStore) Put(ctx context.Context, ch swarm.Chunk) error {
	data := make([]byte, len(ch.Data()))
	copy(data, ch.Data())
	return c.ChunkStore.Put(ctx, swarm.NewChunk(ch.Address(), data))
}

// TestRedundancySingleWriteDistinctShards asserts that every shard of a
// multi-chunk write is a distinct copy rather than an alias of a reused
// producer buffer. Aliased shards make the parities encode duplicated data,
// which stays invisible until a chunk is lost and recovery returns garbage.
func TestRedundancySingleWriteDistinctShards(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := &copyingChunkStore{ChunkStore: inmemchunkstore.New()}

	level := redundancy.MEDIUM
	shardCnt := level.GetMaxShards()
	parityCnt := level.GetParities(shardCnt)

	p := builder.NewPipelineBuilder(ctx, store, false, level)

	// a single Write carrying many chunks is what triggers the buffer reuse
	data := make([]byte, shardCnt*swarm.ChunkSize)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Write(data); err != nil {
		t.Fatal(err)
	}
	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}

	root, err := store.Get(ctx, swarm.NewAddress(sum))
	if err != nil {
		t.Fatal(err)
	}

	refs := root.Data()[swarm.SpanSize:]
	want := shardCnt + parityCnt
	if len(refs)/swarm.HashSize != want {
		t.Fatalf("root holds %d references, want %d", len(refs)/swarm.HashSize, want)
	}

	seen := make(map[string]int, want)
	for i := 0; i < want; i++ {
		ref := string(refs[i*swarm.HashSize : (i+1)*swarm.HashSize])
		if prev, dup := seen[ref]; dup {
			t.Fatalf("reference at slot %d duplicates slot %d: shards are aliased, not copied "+
				"(%d distinct references out of %d)", i, prev, len(seen), want)
		}
		seen[ref] = i
	}
}

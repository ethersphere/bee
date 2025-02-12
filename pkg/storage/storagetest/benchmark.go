// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	valueSize        = flag.Int("value_size", 100, "Size of each value")
	compressionRatio = flag.Float64("compression_ratio", 0.5, "")
	maxConcurrency   = flag.Int("max_concurrency", 2048, "Max concurrency in concurrent benchmark")
	batchSize        = flag.Int("batch_size", 1000, "Max number of records that would trigger commit")
)

var keyLen = 16

const (
	hitKeyFormat     = "1%015d"
	missingKeyFormat = "0%015d"
)

func randomBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = ' ' + byte(r.Intn('~'-' '+1))
	}
	return b
}

func compressibleBytes(r *rand.Rand, ratio float64, valueSize int) []byte {
	m := maxInt(int(float64(valueSize)*ratio), 1)
	p := randomBytes(r, m)
	b := make([]byte, 0, valueSize+valueSize%m)
	for len(b) < valueSize {
		b = append(b, p...)
	}
	return b[:valueSize]
}

type randomValueGenerator struct {
	b []byte
	k int
}

func (g *randomValueGenerator) Value(i int) []byte {
	i = (i * g.k) % len(g.b)
	return g.b[i : i+g.k]
}

func makeRandomValueGenerator(r *rand.Rand, ratio float64, valueSize int) randomValueGenerator {
	b := compressibleBytes(r, ratio, valueSize)
	maxVal := maxInt(valueSize, 1024*1024)
	for len(b) < maxVal {
		b = append(b, compressibleBytes(r, ratio, valueSize)...)
	}
	return randomValueGenerator{b: b, k: valueSize}
}

type entryGenerator interface {
	keyGenerator
	Value(i int) []byte
}

type pairedEntryGenerator struct {
	keyGenerator
	randomValueGenerator
}

type startAtEntryGenerator struct {
	entryGenerator
	start int
}

var _ entryGenerator = (*startAtEntryGenerator)(nil)

func (g *startAtEntryGenerator) NKey() int {
	return g.entryGenerator.NKey() - g.start
}

func (g *startAtEntryGenerator) Key(i int) []byte {
	return g.entryGenerator.Key(g.start + i)
}

func newStartAtEntryGenerator(start int, g entryGenerator) entryGenerator {
	return &startAtEntryGenerator{start: start, entryGenerator: g}
}

func newSequentialKeys(size int, start int, keyFormat string) [][]byte {
	keys := make([][]byte, size)
	buffer := make([]byte, size*keyLen)
	for i := 0; i < size; i++ {
		begin, end := i*keyLen, (i+1)*keyLen
		key := buffer[begin:begin:end]
		_, _ = fmt.Fprintf(bytes.NewBuffer(key), keyFormat, start+i)
		keys[i] = buffer[begin:end:end]
	}
	return keys
}

func newRandomKeys(n int, format string) [][]byte {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	keys := make([][]byte, n)
	buffer := make([]byte, n*keyLen)
	for i := 0; i < n; i++ {
		begin, end := i*keyLen, (i+1)*keyLen
		key := buffer[begin:begin:end]
		_, _ = fmt.Fprintf(bytes.NewBuffer(key), format, r.Intn(n))
		keys[i] = buffer[begin:end:end]
	}
	return keys
}

func newFullRandomKeys(size int, start int, format string) [][]byte {
	keys := newSequentialKeys(size, start, format)
	r := rand.New(rand.NewSource(time.Now().Unix()))
	for i := 0; i < size; i++ {
		j := r.Intn(size)
		keys[i], keys[j] = keys[j], keys[i]
	}
	return keys
}

func newFullRandomEntryGenerator(start, size int) entryGenerator {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	return &pairedEntryGenerator{
		keyGenerator:         newFullRandomKeyGenerator(start, size),
		randomValueGenerator: makeRandomValueGenerator(r, *compressionRatio, *valueSize),
	}
}

func newSequentialEntryGenerator(size int) entryGenerator {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	return &pairedEntryGenerator{
		keyGenerator:         newSequentialKeyGenerator(size),
		randomValueGenerator: makeRandomValueGenerator(r, *compressionRatio, *valueSize),
	}
}

type keyGenerator interface {
	NKey() int
	Key(i int) []byte
}

type reversedKeyGenerator struct {
	keyGenerator
}

var _ keyGenerator = (*reversedKeyGenerator)(nil)

func (g *reversedKeyGenerator) Key(i int) []byte {
	return g.keyGenerator.Key(g.NKey() - i - 1)
}

func newReversedKeyGenerator(g keyGenerator) keyGenerator {
	return &reversedKeyGenerator{keyGenerator: g}
}

type roundKeyGenerator struct {
	keyGenerator
}

var _ keyGenerator = (*roundKeyGenerator)(nil)

func (g *roundKeyGenerator) Key(i int) []byte {
	index := i % g.NKey()
	return g.keyGenerator.Key(index)
}

func newRoundKeyGenerator(g keyGenerator) keyGenerator {
	return &roundKeyGenerator{keyGenerator: g}
}

type predefinedKeyGenerator struct {
	keys [][]byte
}

func (g *predefinedKeyGenerator) NKey() int {
	return len(g.keys)
}

func (g *predefinedKeyGenerator) Key(i int) []byte {
	if i >= len(g.keys) {
		return g.keys[0]
	}
	return g.keys[i]
}

func newRandomKeyGenerator(n int) keyGenerator {
	return &predefinedKeyGenerator{keys: newRandomKeys(n, hitKeyFormat)}
}

func newRandomMissingKeyGenerator(n int) keyGenerator {
	return &predefinedKeyGenerator{keys: newRandomKeys(n, missingKeyFormat)}
}

func newFullRandomKeyGenerator(start, n int) keyGenerator {
	return &predefinedKeyGenerator{keys: newFullRandomKeys(n, start, hitKeyFormat)}
}

func newSequentialKeyGenerator(n int) keyGenerator {
	return &predefinedKeyGenerator{keys: newSequentialKeys(n, 0, hitKeyFormat)}
}

func maxInt(a int, b int) int {
	if a >= b {
		return a
	}
	return b
}

func doRead(b *testing.B, db storage.Store, g keyGenerator, allowNotFound bool) {
	b.Helper()

	for i := 0; i < b.N; i++ {
		key := g.Key(i)
		item := &obj1{
			Id: string(key),
		}
		err := db.Get(item)
		switch {
		case err == nil:
		case allowNotFound && errors.Is(err, storage.ErrNotFound):
		default:
			b.Fatalf("%d: db get key[%s] error: %s\n", b.N, key, err)
		}
	}
}

type singularDBWriter struct {
	db storage.Store
}

func (w *singularDBWriter) Put(key, value []byte) error {
	item := &obj1{
		Id:  string(key),
		Buf: value,
	}
	return w.db.Put(item)
}

func (w *singularDBWriter) Delete(key []byte) error {
	item := &obj1{
		Id: string(key),
	}
	return w.db.Delete(item)
}

func newDBWriter(db storage.Store) *singularDBWriter {
	return &singularDBWriter{db: db}
}

func doWrite(b *testing.B, db storage.Store, g entryGenerator) {
	b.Helper()

	w := newDBWriter(db)
	for i := 0; i < b.N; i++ {
		if err := w.Put(g.Key(i), g.Value(i)); err != nil {
			b.Fatalf("write key '%s': %v", string(g.Key(i)), err)
		}
	}
}

func doDelete(b *testing.B, db storage.Store, g keyGenerator) {
	b.Helper()

	w := newDBWriter(db)
	for i := 0; i < b.N; i++ {
		if err := w.Delete(g.Key(i)); err != nil {
			b.Fatalf("delete key '%s': %v", string(g.Key(i)), err)
		}
	}
}

func resetBenchmark(b *testing.B) {
	b.Helper()

	runtime.GC()
	b.ResetTimer()
}

func populate(b *testing.B, db storage.Store) {
	b.Helper()

	doWrite(b, db, newFullRandomEntryGenerator(0, b.N))
}

// chunk
func doDeleteChunk(b *testing.B, db storage.ChunkStore, g keyGenerator) {
	b.Helper()

	for i := 0; i < b.N; i++ {
		addr := swarm.MustParseHexAddress(string(g.Key(i)))
		if err := db.Delete(context.Background(), addr); err != nil {
			b.Fatalf("delete key '%s': %v", string(g.Key(i)), err)
		}
	}
}

func doWriteChunk(b *testing.B, db storage.Putter, g entryGenerator) {
	b.Helper()

	for i := 0; i < b.N; i++ {
		buf := make([]byte, swarm.HashSize)
		if _, err := hex.Decode(buf, g.Key(i)); err != nil {
			b.Fatalf("decode value: %v", err)
		}
		addr := swarm.NewAddress(buf)
		chunk := swarm.NewChunk(addr, g.Value(i)).WithStamp(postagetesting.MustNewStamp())
		if err := db.Put(context.Background(), chunk); err != nil {
			b.Fatalf("write key '%s': %v", string(g.Key(i)), err)
		}
	}
}

func doReadChunk(b *testing.B, db storage.ChunkStore, g keyGenerator, allowNotFound bool) {
	b.Helper()

	for i := 0; i < b.N; i++ {
		key := string(g.Key(i))
		addr := swarm.MustParseHexAddress(key)
		_, err := db.Get(context.Background(), addr)
		switch {
		case err == nil:
		case allowNotFound && errors.Is(err, storage.ErrNotFound):
		default:
			b.Fatalf("%d: db get key[%s] error: %s\n", b.N, key, err)
		}
	}
}

// fixed size batch
type batchDBWriter struct {
	db    storage.Batcher
	batch storage.Batch
	max   int
	count int
}

func (w *batchDBWriter) commit(maxValue int) {
	if w.count >= maxValue {
		_ = w.batch.Commit()
		w.count = 0
		w.batch = w.db.Batch(context.Background())
	}
}

func (w *batchDBWriter) Put(key, value []byte) {
	item := &obj1{
		Id:  string(key),
		Buf: value,
	}
	_ = w.batch.Put(item)
	w.count++
	w.commit(w.max)
}

func (w *batchDBWriter) Delete(key []byte) {
	item := &obj1{
		Id: string(key),
	}
	_ = w.batch.Delete(item)
	w.count++
	w.commit(w.max)
}

func newBatchDBWriter(db storage.Batcher) *batchDBWriter {
	batch := db.Batch(context.Background())
	return &batchDBWriter{
		db:    db,
		batch: batch,
		max:   *batchSize,
	}
}

package storetesting

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"testing"
	"time"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

var (
	valueSize        = flag.Int("value_size", 100, "Size of each value")
	compressionRatio = flag.Float64("compression_ratio", 0.5, "")
)

const (
	hitKeyFormat     = "%016d+"
	missingKeyFormat = "%016d-"
)

func randomBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = ' ' + byte(r.Intn('~'-' '+1))
	}
	return b
}

func compressibleBytes(r *rand.Rand, ratio float64, n int) []byte {
	m := maxInt(int(float64(n)*ratio), 1)
	p := randomBytes(r, m)
	b := make([]byte, 0, n+n%m)
	for len(b) < n {
		b = append(b, p...)
	}
	return b[:n]
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
	max := maxInt(valueSize, 1024*1024)
	for len(b) < max {
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

func newSequentialKeys(n int, start int, keyFormat string) [][]byte {
	keys := make([][]byte, n)
	buffer := make([]byte, n*keyLen)
	for i := 0; i < n; i++ {
		begin, end := i*keyLen, (i+1)*keyLen
		key := buffer[begin:begin:end]
		n, _ := fmt.Fprintf(bytes.NewBuffer(key), keyFormat, start+i)
		if n != keyLen {
			panic("n != keyLen")
		}
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
		n, _ := fmt.Fprintf(bytes.NewBuffer(key), format, r.Intn(n))
		if n != keyLen {
			panic("n != keyLen")
		}
		keys[i] = buffer[begin:end:end]
	}
	return keys
}

func newFullRandomKeys(n int, start int, format string) [][]byte {
	keys := newSequentialKeys(n, start, format)
	r := rand.New(rand.NewSource(time.Now().Unix()))
	for i := 0; i < n; i++ {
		j := r.Intn(n)
		keys[i], keys[j] = keys[j], keys[i]
	}
	return keys
}

func newFullRandomEntryGenerator(start, n int) entryGenerator {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	return &pairedEntryGenerator{
		keyGenerator:         newFullRandomKeyGenerator(start, n),
		randomValueGenerator: makeRandomValueGenerator(r, *compressionRatio, *valueSize),
	}
}

func newSequentialEntryGenerator(n int) entryGenerator {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	return &pairedEntryGenerator{
		keyGenerator:         newSequentialKeyGenerator(n),
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
	return g.keyGenerator.Key(i % g.NKey())
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

func newSortedRandomKeyGenerator(n int) keyGenerator {
	keys := newRandomKeys(n, hitKeyFormat)
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i], keys[j]) < 0
	})
	return &predefinedKeyGenerator{keys: keys}
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

func (w *singularDBWriter) Put(key, value []byte) {
	item := &obj1{
		Id:  string(key),
		Buf: value,
	}
	err := w.db.Put(item)
	if err != nil {
		panic(err)
	}
}

func (w *singularDBWriter) Delete(key []byte) {
	item := &obj1{
		Id: string(key),
	}
	err := w.db.Delete(item)
	if err != nil {
		panic(err)
	}
}

func newDBWriter(db storage.Store) *singularDBWriter {
	return &singularDBWriter{db: db}
}

func doWrite(db storage.Store, n int, g entryGenerator) {
	w := newDBWriter(db)
	for i := 0; i < n; i++ {
		w.Put(g.Key(i), g.Value(i))
	}
}

func doDelete(b *testing.B, db storage.Store, g keyGenerator) {
	w := newDBWriter(db)
	for i := 0; i < b.N; i++ {
		w.Delete(g.Key(i))
	}
}

func resetBenchmark(b *testing.B) {
	runtime.GC()
	b.ResetTimer()
}

func populate(n int, db storage.Store) {
	doWrite(db, n, newFullRandomEntryGenerator(0, n))
}

// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

var (
	// MinAddressBytes represents bytes that can be used to represent a min. address.
	MinAddressBytes = [swarm.HashSize]byte{swarm.HashSize - 1: 0x00}

	// MaxAddressBytes represents bytes that can be used to represent a max. address.
	MaxAddressBytes = [swarm.HashSize]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
)

var _ storage.Item = (*ItemStub)(nil)

// ItemStub is a stub for storage.Item.
type ItemStub struct {
	MarshalBuf   []byte
	MarshalErr   error
	UnmarshalBuf []byte
}

// ID implements the storage.Item interface.
func (im ItemStub) ID() string { return fmt.Sprintf("%+v", im) }

// Namespace implements the storage.Item interface.
func (im ItemStub) Namespace() string { return "test" }

// Marshal implements the storage.Item interface.
func (im ItemStub) Marshal() ([]byte, error) {
	return im.MarshalBuf, im.MarshalErr
}

// Unmarshal implements the storage.Item interface.
func (im *ItemStub) Unmarshal(data []byte) error {
	im.UnmarshalBuf = data
	return nil
}

type obj1 struct {
	Id      string
	SomeInt uint64
	Buf     []byte
}

func (obj1) Namespace() string { return "obj1" }

func (o *obj1) ID() string { return o.Id }

// ID is 32 bytes
func (o *obj1) Marshal() ([]byte, error) {
	buf := make([]byte, 40)
	copy(buf[:32], []byte(o.Id))
	binary.LittleEndian.PutUint64(buf[32:], o.SomeInt)
	buf = append(buf, o.Buf[:]...)
	return buf, nil
}

func (o *obj1) Unmarshal(buf []byte) error {
	if len(buf) < 40 {
		return errors.New("invalid length")
	}
	o.Id = strings.TrimRight(string(buf[:32]), string([]byte{0}))
	o.SomeInt = binary.LittleEndian.Uint64(buf[32:])
	o.Buf = buf[40:]
	return nil
}

type obj2 struct {
	Id        int
	SomeStr   string
	SomeFloat float64
}

func (obj2) Namespace() string { return "obj2" }

func (o *obj2) ID() string { return strconv.Itoa(o.Id) }

func (o *obj2) Marshal() ([]byte, error) { return json.Marshal(o) }

func (o *obj2) Unmarshal(buf []byte) error { return json.Unmarshal(buf, o) }

func randBytes(count int) []byte {
	buf := make([]byte, count)
	_, _ = rand.Read(buf)
	return buf
}

func checkTestItemEqual(t *testing.T, a, b storage.Item) {
	t.Helper()

	if a.Namespace() != b.Namespace() {
		t.Fatalf("namespace doesnt match %s and %s", a.Namespace(), b.Namespace())
	}

	if a.ID() != b.ID() {
		t.Fatalf("ID doesnt match %s and %s %d %d", a.ID(), b.ID(), len(a.ID()), len(b.ID()))
	}

	buf1, err := a.Marshal()
	if err != nil {
		t.Fatalf("failed marshaling: %v", err)
	}

	buf2, err := b.Marshal()
	if err != nil {
		t.Fatalf("failed marshaling: %v", err)
	}

	if !bytes.Equal(buf1, buf2) {
		t.Fatalf("bytes not equal for item %s/%s", a.Namespace(), a.ID())
	}
}

// TestStore provides correctness testsuite for Store interface.
func TestStore(t *testing.T, s storage.Store) {
	t.Helper()

	testObjs := []storage.Item{
		&obj1{
			Id:      "aaaaaaaaaaa",
			SomeInt: 3,
			Buf:     randBytes(128),
		},
		&obj1{
			Id:      "bbbbbbbbbbb",
			SomeInt: 4,
			Buf:     randBytes(64),
		},
		&obj1{
			Id:      "ccccccccccc",
			SomeInt: 5,
			Buf:     randBytes(32),
		},
		&obj1{
			Id:      "ddddddddddd",
			SomeInt: 6,
			Buf:     randBytes(256),
		},
		&obj1{
			Id:      "dddddeeeeee",
			SomeInt: 7,
			Buf:     randBytes(16),
		},
		&obj2{
			Id:        1,
			SomeStr:   "asdasdasdasdasd",
			SomeFloat: 10000.00001,
		},
		&obj2{
			Id:        2,
			SomeStr:   "dfgdfgdfgdfgdfg",
			SomeFloat: 200001.11123,
		},
		&obj2{
			Id:        3,
			SomeStr:   "qweqweqweqweqwe",
			SomeFloat: 1223444.1122,
		},
		&obj2{
			Id:        4,
			SomeStr:   "",
			SomeFloat: 1.0,
		},
		&obj2{
			Id:        5,
			SomeStr:   "abc",
			SomeFloat: 121213123111.112333,
		},
	}

	t.Run("create new entries", func(t *testing.T) {
		for _, i := range testObjs {
			err := s.Put(i)
			if err != nil {
				t.Fatalf("failed to add new entry: %v", err)
			}
		}
	})

	t.Run("has entries", func(t *testing.T) {
		for _, i := range testObjs {
			found, err := s.Has(i)
			if err != nil {
				t.Fatalf("failed to check entry: %v", err)
			}
			if !found {
				t.Fatalf("expected entry to be found %s/%s", i.Namespace(), i.ID())
			}
		}
	})

	t.Run("get entries", func(t *testing.T) {
		for idx, i := range testObjs {
			var readObj storage.Item
			if idx < 5 {
				readObj = &obj1{Id: i.ID()}
			} else {
				readObj = &obj2{Id: i.(*obj2).Id}
			}

			err := s.Get(readObj)
			if err != nil {
				t.Fatalf("failed to get obj %s/%s", readObj.Namespace(), readObj.ID())
			}

			checkTestItemEqual(t, readObj, i)
		}
	})

	t.Run("get size", func(t *testing.T) {
		for idx, i := range testObjs {
			var readObj storage.Item
			if idx < 5 {
				readObj = &obj1{Id: i.ID()}
			} else {
				readObj = &obj2{Id: i.(*obj2).Id}
			}

			sz, err := s.GetSize(readObj)
			if err != nil {
				t.Fatalf("failed to get obj %s/%s", readObj.Namespace(), readObj.ID())
			}

			buf, err := i.Marshal()
			if err != nil {
				t.Fatalf("failed marshaling test item: %v", err)
			}

			if sz != len(buf) {
				t.Fatalf("sizes dont match %s/%s expected %d found %d", i.Namespace(), i.ID(), len(buf), sz)
			}
		}
	})

	t.Run("count", func(t *testing.T) {
		t.Run("obj1", func(t *testing.T) {
			count1, err := s.Count(&obj1{})
			if err != nil {
				t.Fatalf("failed getting count: %v", err)
			}
			if count1 != 5 {
				t.Fatalf("unexpected count exp 5 found %d", count1)
			}
		})
		t.Run("obj2", func(t *testing.T) {
			count2, err := s.Count(&obj2{})
			if err != nil {
				t.Fatalf("failed getting count: %v", err)
			}
			if count2 != 5 {
				t.Fatalf("unexpected count exp 5 found %d", count2)
			}
		})
	})

	t.Run("iterate ascending", func(t *testing.T) {
		t.Run("obj1", func(t *testing.T) {
			idx := 0
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj1) },
				ItemAttribute: storage.QueryItem,
			}, func(r storage.Result) (bool, error) {
				checkTestItemEqual(t, r.Entry, testObjs[idx])
				idx++
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration: %v", err)
			}
			if idx != 5 {
				t.Fatalf("unexpected no of entries in iteration exp 5 found %d", idx)
			}
		})
		t.Run("obj2", func(t *testing.T) {
			idx := 5
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj2) },
				ItemAttribute: storage.QueryItem,
			}, func(r storage.Result) (bool, error) {
				checkTestItemEqual(t, r.Entry, testObjs[idx])
				idx++
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration: %v", err)
			}
			if idx != 10 {
				t.Fatalf("unexpected no of entries in iteration exp 5 found %d", idx-5)
			}
		})
	})

	t.Run("iterate descending", func(t *testing.T) {
		t.Run("obj1", func(t *testing.T) {
			idx := 4
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj1) },
				ItemAttribute: storage.QueryItem,
				Order:         storage.KeyDescendingOrder,
			}, func(r storage.Result) (bool, error) {
				if idx < 0 {
					t.Fatal("index overflow")
				}
				checkTestItemEqual(t, r.Entry, testObjs[idx])
				idx--
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration: %v", err)
			}
			if idx != -1 {
				t.Fatalf("unexpected no of entries in iteration exp 5 found %d", 4-idx)
			}
		})
		t.Run("obj2", func(t *testing.T) {
			idx := 9
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj2) },
				ItemAttribute: storage.QueryItem,
				Order:         storage.KeyDescendingOrder,
			}, func(r storage.Result) (bool, error) {
				if idx < 5 {
					t.Fatal("index overflow")
				}
				checkTestItemEqual(t, r.Entry, testObjs[idx])
				idx--
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration: %v", err)
			}
			if idx != 4 {
				t.Fatalf("unexpected no of entries in iteration exp 5 found %d", 9-idx)
			}
		})
	})

	t.Run("iterate attribute", func(t *testing.T) {
		t.Run("key only", func(t *testing.T) {
			idx := 0
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj1) },
				ItemAttribute: storage.QueryItemID,
			}, func(r storage.Result) (bool, error) {
				if r.Entry != nil {
					t.Fatal("expected entry to be nil")
				}
				if r.ID != testObjs[idx].ID() {
					t.Fatalf("invalid key order expected %s found %s", testObjs[idx].ID(), r.ID)
				}
				idx++
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration %v", err)
			}
			if idx != 5 {
				t.Fatalf("unexpected no of entries in iteration exp 5 found %d", idx)
			}
		})
		t.Run("size only", func(t *testing.T) {
			idx := 9
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj2) },
				ItemAttribute: storage.QueryItemSize,
				Order:         storage.KeyDescendingOrder,
			}, func(r storage.Result) (bool, error) {
				if idx < 5 {
					t.Fatal("index overflow")
				}
				if r.Entry != nil {
					t.Fatal("expected entry to be nil")
				}
				if r.ID != testObjs[idx].ID() {
					t.Fatalf("invalid key order expected %s found %s", testObjs[idx].ID(), r.ID)
				}
				buf, err := testObjs[idx].Marshal()
				if err != nil {
					t.Fatalf("failed marshaling: %v", err)
				}
				if r.Size != len(buf) {
					t.Fatalf("incorrect size in query expected %d found %d  id %s", len(buf), r.Size, r.ID)
				}
				idx--
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration: %v", err)
			}
			if idx != 4 {
				t.Fatalf("unexpected no of entries in iteration exp 5 found %d", 9-idx)
			}
		})
	})

	t.Run("iterate filters", func(t *testing.T) {
		idx := 2
		err := s.Iterate(storage.Query{
			Factory:       func() storage.Item { return new(obj1) },
			ItemAttribute: storage.QueryItem,
			Filters: []storage.Filter{
				func(_ string, v []byte) bool {
					return binary.LittleEndian.Uint64(v[32:]) < 5
				},
			},
		}, func(r storage.Result) (bool, error) {
			checkTestItemEqual(t, r.Entry, testObjs[idx])
			idx++
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if idx != 5 {
			t.Fatalf("unexpected no of entries in iteration exp 3 found %d", idx-2)
		}
	})

	t.Run("delete", func(t *testing.T) {
		for idx, i := range testObjs {
			if idx < 3 || idx > 7 {
				err := s.Delete(i)
				if err != nil {
					t.Fatalf("failed deleting entry: %v", err)
				}
				found, err := s.Has(i)
				if err != nil {
					t.Fatalf("unexpected error in has: %v", err)
				}
				if found {
					t.Fatalf("found id %s, expected to not be found", i.ID())
				}
				if idx < 3 {
					err = s.Get(&obj1{Id: i.ID()})
				} else {
					err = s.Get(&obj2{Id: i.(*obj2).Id})
				}
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatal("expected storage.NotFound error")
				}
				if idx < 3 {
					_, err = s.GetSize(&obj1{Id: i.ID()})
				} else {
					_, err = s.GetSize(&obj2{Id: i.(*obj2).Id})
				}
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatal("expected storage.NotFound error")
				}
			}
		}
	})

	t.Run("count after delete", func(t *testing.T) {
		t.Run("obj1", func(t *testing.T) {
			count1, err := s.Count(&obj1{})
			if err != nil {
				t.Fatalf("failed getting count: %v", err)
			}
			if count1 != 2 {
				t.Fatalf("unexpected count exp 2 found %d", count1)
			}
		})
		t.Run("obj2", func(t *testing.T) {
			count2, err := s.Count(&obj2{})
			if err != nil {
				t.Fatalf("failed getting count: %v", err)
			}
			if count2 != 3 {
				t.Fatalf("unexpected count exp 3 found %d", count2)
			}
		})
	})

	t.Run("iterate after delete", func(t *testing.T) {
		t.Run("obj1", func(t *testing.T) {
			idx := 3
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj1) },
				ItemAttribute: storage.QueryItem,
			}, func(r storage.Result) (bool, error) {
				checkTestItemEqual(t, r.Entry, testObjs[idx])
				idx++
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration: %v", err)
			}
			if idx != 5 {
				t.Fatalf("unexpected no of entries in iteration exp 2 found %d", idx-3)
			}
		})
		t.Run("obj2", func(t *testing.T) {
			idx := 5
			err := s.Iterate(storage.Query{
				Factory:       func() storage.Item { return new(obj2) },
				ItemAttribute: storage.QueryItem,
			}, func(r storage.Result) (bool, error) {
				checkTestItemEqual(t, r.Entry, testObjs[idx])
				idx++
				return false, nil
			})
			if err != nil {
				t.Fatalf("unexpected error while iteration: %v", err)
			}
			if idx != 8 {
				t.Fatalf("unexpected no of entries in iteration exp 3 found %d", idx-5)
			}
		})
	})

	t.Run("error during iteration", func(t *testing.T) {
		expErr := errors.New("test error")
		err := s.Iterate(storage.Query{
			Factory:       func() storage.Item { return new(obj1) },
			ItemAttribute: storage.QueryItem,
		}, func(r storage.Result) (bool, error) {
			return true, expErr
		})
		if !errors.Is(err, expErr) {
			t.Fatal("incorrect error returned")
		}
	})

	t.Run("close", func(t *testing.T) {
		err := s.Close()
		if err != nil {
			t.Fatalf("failed closing: %v", err)
		}
	})
}

// ItemMarshalAndUnmarshalTest represents a test case
// for the TestItemMarshalAndUnmarshal function.
type ItemMarshalAndUnmarshalTest struct {
	Item         storage.Item
	Factory      func() storage.Item
	MarshalErr   error // Expected error from Marshal.
	UnmarshalErr error // Expected error from Unmarshal.
}

// TestItemMarshalAndUnmarshal provides correctness testsuite
// for storage.Item serialization and deserialization.
func TestItemMarshalAndUnmarshal(t *testing.T, test *ItemMarshalAndUnmarshalTest) {
	t.Helper()

	buf, err := test.Item.Marshal()
	if !errors.Is(err, test.MarshalErr) {
		t.Fatalf("Marshal(): want error: %v; have error %v", test.MarshalErr, err)
	}
	if test.MarshalErr != nil {
		return
	}
	if len(buf) == 0 {
		t.Fatalf("Marshal(): empty buffer")
	}

	item2 := test.Factory()
	if err := item2.Unmarshal(buf); !errors.Is(err, test.UnmarshalErr) {
		t.Fatalf("Unmarshal(): want error: %v; have error %v", test.UnmarshalErr, err)
	}
	if test.UnmarshalErr != nil {
		return
	}

	want, have := test.Item, item2
	if diff := cmp.Diff(want, have); diff != "" {
		t.Errorf("Marshal/Unmarshal mismatch (-want +have):\n%s", diff)
	}
}

func RunStoreBenchmarkTests(b *testing.B, s storage.Store) {
	b.Run("WriteSequential", func(b *testing.B) {
		BenchmarkWriteSequential(b, s)
	})
	b.Run("WriteInBatches", func(b *testing.B) {
		BenchmarkWriteInBatches(b, s)
	})
	b.Run("WriteInFixedSizeBatches", func(b *testing.B) {
		BenchmarkWriteInFixedSizeBatches(b, s)
	})
	b.Run("WriteRandom", func(b *testing.B) {
		BenchmarkWriteRandom(b, s)
	})
	b.Run("ReadSequential", func(b *testing.B) {
		BenchmarkReadSequential(b, s)
	})
	b.Run("ReadRandom", func(b *testing.B) {
		BenchmarkReadRandom(b, s)
	})
	b.Run("ReadRandomMissing", func(b *testing.B) {
		BenchmarkReadRandomMissing(b, s)
	})
	b.Run("ReadReverse", func(b *testing.B) {
		BenchmarkReadReverse(b, s)
	})
	b.Run("ReadRedHot", func(b *testing.B) {
		BenchmarkReadHot(b, s)
	})
	b.Run("IterateSequential", func(b *testing.B) {
		BenchmarkIterateSequential(b, s)
	})
	b.Run("IterateReverse", func(b *testing.B) {
		BenchmarkIterateReverse(b, s)
	})
	b.Run("DeleteRandom", func(b *testing.B) {
		BenchmarkDeleteRandom(b, s)
	})
	b.Run("DeleteSequential", func(b *testing.B) {
		BenchmarkDeleteSequential(b, s)
	})
	b.Run("DeleteInBatches", func(b *testing.B) {
		BenchmarkDeleteInBatches(b, s)
	})
	b.Run("DeleteInFixedSizeBatches", func(b *testing.B) {
		BenchmarkDeleteInFixedSizeBatches(b, s)
	})
}

func BenchmarkReadRandom(b *testing.B, db storage.Store) {
	g := newRandomKeyGenerator(b.N)
	resetBenchmark(b)
	doRead(b, db, g, false)
}

func BenchmarkReadRandomMissing(b *testing.B, db storage.Store) {
	g := newRandomMissingKeyGenerator(b.N)
	resetBenchmark(b)
	doRead(b, db, g, true)
}

func BenchmarkReadSequential(b *testing.B, db storage.Store) {
	g := newSequentialKeyGenerator(b.N)
	populate(b, db)
	resetBenchmark(b)
	doRead(b, db, g, false)
}

func BenchmarkReadReverse(b *testing.B, db storage.Store) {
	g := newReversedKeyGenerator(newSequentialKeyGenerator(b.N))
	populate(b, db)
	resetBenchmark(b)
	doRead(b, db, g, false)
}

func BenchmarkReadHot(b *testing.B, db storage.Store) {
	k := maxInt((b.N+99)/100, 1)
	g := newRoundKeyGenerator(newRandomKeyGenerator(k))
	populate(b, db)
	resetBenchmark(b)
	doRead(b, db, g, false)
}

func BenchmarkIterateSequential(b *testing.B, db storage.Store) {
	populate(b, db)
	resetBenchmark(b)
	var counter int
	fn := func(r storage.Result) (bool, error) {
		counter++
		if counter > b.N {
			return true, nil
		}
		return false, nil
	}
	q := storage.Query{
		Factory: func() storage.Item { return new(obj1) },
		Order:   storage.KeyAscendingOrder,
	}
	if err := db.Iterate(q, fn); err != nil {
		b.Fatal("iterate", err)
	}
}

func BenchmarkIterateReverse(b *testing.B, db storage.Store) {
	populate(b, db)
	resetBenchmark(b)
	var counter int
	fn := func(storage.Result) (bool, error) {
		counter++
		if counter > b.N {
			return true, nil
		}
		return false, nil
	}
	q := storage.Query{
		Factory: func() storage.Item { return new(obj1) },
		Order:   storage.KeyDescendingOrder,
	}
	if err := db.Iterate(q, fn); err != nil {
		b.Fatal("iterate", err)
	}
}

func BenchmarkWriteSequential(b *testing.B, db storage.Store) {
	g := newSequentialEntryGenerator(b.N)
	resetBenchmark(b)
	doWrite(b, db, g)
}

func BenchmarkWriteInBatches(b *testing.B, db storage.Store) {
	g := newSequentialEntryGenerator(b.N)
	batch, _ := db.Batch(context.Background())
	resetBenchmark(b)
	for i := 0; i < b.N; i++ {
		key := g.Key(i)
		item := &obj1{
			Id:  string(key),
			Buf: g.Value(i),
		}
		if err := batch.Put(item); err != nil {
			b.Fatalf("write key '%s': %v", string(g.Key(i)), err)
		}
	}
	if err := batch.Commit(); err != nil {
		b.Fatal("commit batch", err)
	}
}

func BenchmarkWriteInFixedSizeBatches(b *testing.B, db storage.Store) {
	g := newSequentialEntryGenerator(b.N)
	writer := newBatchDBWriter(db)
	resetBenchmark(b)
	for i := 0; i < b.N; i++ {
		writer.Put(g.Key(i), g.Value(i))
	}
}

func BenchmarkWriteRandom(b *testing.B, db storage.Store) {
	for i, n := 1, *maxConcurrency; i <= n; i *= 2 {
		name := fmt.Sprintf("parallelism-%d", i)
		runtime.GC()
		parallelism := i
		b.Run(name, func(b *testing.B) {
			var gens []entryGenerator
			start, step := 0, (b.N+parallelism)/parallelism
			n := step * parallelism
			g := newFullRandomEntryGenerator(0, n)
			for i := 0; i < parallelism; i++ {
				gens = append(gens, newStartAtEntryGenerator(start, g))
				start += step
			}
			resetBenchmark(b)
			var wg sync.WaitGroup
			wg.Add(len(gens))
			for _, g := range gens {
				go func(g entryGenerator) {
					defer wg.Done()
					doWrite(b, db, g)
				}(g)
			}
			wg.Wait()
		})
	}
}

func BenchmarkDeleteRandom(b *testing.B, db storage.Store) {
	g := newFullRandomEntryGenerator(0, b.N)
	doWrite(b, db, g)
	resetBenchmark(b)
	doDelete(b, db, g)
}

func BenchmarkDeleteSequential(b *testing.B, db storage.Store) {
	g := newSequentialEntryGenerator(b.N)
	doWrite(b, db, g)
	resetBenchmark(b)
	doDelete(b, db, g)
}

func BenchmarkDeleteInBatches(b *testing.B, db storage.Store) {
	g := newSequentialEntryGenerator(b.N)
	doWrite(b, db, g)
	resetBenchmark(b)
	batch, _ := db.Batch(context.Background())
	for i := 0; i < b.N; i++ {
		item := &obj1{
			Id: string(g.Key(i)),
		}
		if err := batch.Delete(item); err != nil {
			b.Fatalf("delete key '%s': %v", string(g.Key(i)), err)
		}
	}
	if err := batch.Commit(); err != nil {
		b.Fatal("commit batch", err)
	}
}

func BenchmarkDeleteInFixedSizeBatches(b *testing.B, db storage.Store) {
	g := newSequentialEntryGenerator(b.N)
	doWrite(b, db, g)
	resetBenchmark(b)
	writer := newBatchDBWriter(db)
	for i := 0; i < b.N; i++ {
		writer.Delete(g.Key(i))
	}
}

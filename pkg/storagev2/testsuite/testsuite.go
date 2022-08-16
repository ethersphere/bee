// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storetesting

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

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

func RunCorrectnessTests(t *testing.T, s storage.Store) {
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

// packages using the store would define Items and define the Serializable interface
// on them. They could use these tests to test the serialization part.
func RunItemSerializationTests(t *testing.T, i storage.Item, factory func() storage.Item) {
	t.Helper()

	t.Run("marshal", func(t *testing.T) {
		buf1, err := i.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling: %v", err)
		}

		if buf1 == nil || len(buf1) <= 0 {
			t.Fatal("marshaling produced nil buffer")
		}

		buf2, err := i.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling: %v", err)
		}

		if !bytes.Equal(buf1, buf2) {
			t.Fatal("marshaling twice produced different result")
		}
	})

	t.Run("marshal then unmarshal", func(t *testing.T) {
		buf1, err := i.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling: %v", err)
		}

		i2 := factory()
		err = i2.Unmarshal(buf1)
		if err != nil {
			t.Fatalf("failed unmarshaling: %v", err)
		}

		if !reflect.DeepEqual(i, i2) {
			t.Fatal("new item is not equal to old one")
		}

		buf2, err := i2.Marshal()
		if err != nil {
			t.Fatalf("failed marshaling new item: %v", err)
		}

		if !bytes.Equal(buf1, buf2) {
			t.Fatal("marshaling new item produced different result")
		}
	})
}

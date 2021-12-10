package indigo_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/indigo"
	"github.com/ethersphere/bee/pkg/indigo/pot"
)

var _ pot.Entry = (*mockEntry)(nil)

type mockEntry struct {
	key []byte
	val int
}

var maxVal = 100

func newMockEntry(t *testing.T) *mockEntry {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	val := rand.Intn(maxVal)
	return &mockEntry{key: key, val: val}
}

func (m *mockEntry) Key() []byte {
	return m.key
}

func (m *mockEntry) String() string {
	return fmt.Sprintf("%d", m.val)
}

func (m *mockEntry) Equal(n pot.Entry) bool {
	return m.val == n.(*mockEntry).val
}

func eq(m, n *mockEntry) bool {
	return bytes.Equal(m.key, n.key) && m.Equal(n)
}

func (m *mockEntry) MarshalBinary() ([]byte, error) {
	buf := make([]byte, len(m.key)+4)
	copy(buf, m.key)
	binary.BigEndian.PutUint32(buf[len(m.key):], uint32(m.val))
	return buf, nil
}

func (m *mockEntry) UnmarshalBinary(buf []byte) error {
	m.key = make([]byte, len(buf)-4)
	copy(m.key, buf)
	m.val = int(binary.BigEndian.Uint32(buf[len(m.key):]))
	return nil
}

func TestUpdateCorrectness(t *testing.T) {
	idx, err := indigo.New(t.TempDir(), "test", func() pot.Entry { return &mockEntry{} }, &pot.SingleOrder{})
	if err != nil {
		t.Fatal(err)
	}
	want := newDetMockEntry(0)
	want2 := newDetMockEntry(1)
	ctx := context.Background()
	t.Run("not found on empty index", func(t *testing.T) {
		checkNotFound(t, ctx, idx, want)
	})
	t.Run("add item to empty index and find it", func(t *testing.T) {
		idx.Add(ctx, want)
		checkFound(t, ctx, idx, want)
	})
	t.Run("add same item and find no change", func(t *testing.T) {
		idx.Add(ctx, want)
		checkFound(t, ctx, idx, want)
	})
	t.Run("delete item and not find it", func(t *testing.T) {
		idx.Delete(ctx, want.Key())
		checkNotFound(t, ctx, idx, want)
	})
	t.Run("add 2 items to empty index and find them", func(t *testing.T) {
		idx.Add(ctx, want)
		checkFound(t, ctx, idx, want)
		idx.Add(ctx, want2)
		checkFound(t, ctx, idx, want)
		checkFound(t, ctx, idx, want2)
	})
	t.Run("delete first item and not find it", func(t *testing.T) {
		idx.Delete(ctx, want.Key())
		checkNotFound(t, ctx, idx, want)
		checkFound(t, ctx, idx, want2)
	})
	t.Run("once again add first item and find both", func(t *testing.T) {
		idx.Add(ctx, want)
		checkFound(t, ctx, idx, want2)
		checkFound(t, ctx, idx, want)
	})
	t.Run("delete latest added item and find only item 2", func(t *testing.T) {
		idx.Delete(ctx, want.Key())
		checkFound(t, ctx, idx, want2)
		checkNotFound(t, ctx, idx, want)
	})
	wantMod := &mockEntry{key: want.key, val: want.val + 1}
	want2Mod := &mockEntry{key: want2.key, val: want2.val + 1}
	t.Run("modify item", func(t *testing.T) {
		idx.Add(ctx, want)
		checkFound(t, ctx, idx, want)
		checkFound(t, ctx, idx, want2)
		idx.Add(ctx, wantMod)
		checkFound(t, ctx, idx, wantMod)
		checkFound(t, ctx, idx, want2)
		idx.Add(ctx, want2Mod)
		checkFound(t, ctx, idx, wantMod)
		checkFound(t, ctx, idx, want2Mod)
	})
}

func TestEdgeCasesCorrectness(t *testing.T) {

	ctx := context.Background()

	t.Run("not found on empty index", func(t *testing.T) {
		idx, err := indigo.New(t.TempDir(), "test", func() pot.Entry { return &mockEntry{} }, &pot.SingleOrder{})
		if err != nil {
			t.Fatal(err)
		}
		ints := []int{0, 1, 2}
		entries := make([]*mockEntry, 3)
		for i, j := range ints {
			entry := newDetMockEntry(j)
			idx.Add(ctx, entry)
			entries[i] = entry
		}
		idx.Delete(ctx, entries[1].Key())
		checkNotFound(t, ctx, idx, entries[1])
		checkFound(t, ctx, idx, entries[2])
	})
	t.Run("not found on empty index", func(t *testing.T) {
		idx, err := indigo.New(t.TempDir(), "test", func() pot.Entry { return &mockEntry{} }, &pot.SingleOrder{})
		if err != nil {
			t.Fatal(err)
		}
		ints := []int{5, 4, 7, 8}
		entries := make([]*mockEntry, 4)
		for i, j := range ints {
			entry := newDetMockEntry(j)
			idx.Add(ctx, entry)
			entries[i] = entry
		}
		idx.Delete(ctx, entries[1].Key())
		checkFound(t, ctx, idx, entries[2])
		checkFound(t, ctx, idx, entries[0])
		checkFound(t, ctx, idx, entries[3])
	})
}

func TestIterate(t *testing.T) {
	count := 20
	idx, err := indigo.New(t.TempDir(), "test", func() pot.Entry { return &mockEntry{} }, &pot.SingleOrder{})
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for i := 0; i < count; i++ {
		idx.Add(ctx, newDetMockEntry(i))
		n := 0
		idx.Iter(func(e pot.Entry) { n++ })
		if n != i+1 {
			t.Fatalf("incorrect number of items. want %d, got %d", i+1, n)
		}
	}
}

type testIndex struct {
	mu *sync.Mutex
	*indigo.Index
	errc    chan error
	deleted map[int]struct{}
}

func TestConcurrency(t *testing.T) {
	count := 1000
	index, err := indigo.New(t.TempDir(), "test", func() pot.Entry { return &mockEntry{} }, &pot.SingleOrder{})
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()

	idx := &testIndex{
		mu:      &sync.Mutex{},
		Index:   index,
		deleted: make(map[int]struct{}),
		errc:    make(chan error),
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	top := make(chan int)
	go func() {
		defer close(top)
		for i := 0; i < count; i++ {
			idx.Add(ctx, newDetMockEntry(i))
			if i%5 == 4 {
				select {
				case top <- i:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	j := 0
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		max, more := <-top
		for more {
			j++
			wg.Add(1)
			go func(max int) {
				defer wg.Done()
				if err := idx.findRandomEntry(ctx, max); err != nil {
					select {
					case idx.errc <- err:
					case <-ctx.Done():
					}
					return
				}
			}(max)
			select {
			case max, more = <-top:
				if max > 0 {
					idx.deleteRandomEntry(ctx, max)
				}
			default:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(idx.errc)
	}()
	select {
	case err = <-idx.errc:
	case <-ctx.Done():
		err = ctx.Err()
	}
	t.Logf("processed %d retrievals\n", j)
	if err != nil {
		t.Fatal(err)
	}
}

func (idx *testIndex) String() string {
	s := fmt.Sprintln("deleted", idx.deleted)
	idx.Iter(func(e pot.Entry) { s = fmt.Sprintf("%s %d", s, e.(*mockEntry).val) })
	return s
}

func (idx *testIndex) deleteRandomEntry(ctx context.Context, max int) {
	n := int(rand.Int31n(int32(max)))
	want := newDetMockEntry(n)
	idx.mu.Lock()
	idx.deleted[n] = struct{}{}
	idx.Delete(ctx, want.key)
	for m := 0; m < max; m++ {
		want := newDetMockEntry(m)
		_, found := idx.deleted[m]
		_, err := idx.Find(ctx, want.key)
		if errors.Is(err, pot.ErrNotFound) && !found {
			panic(fmt.Sprintf("item %d not found in store\n%s\n", m, idx.Root()))
		}
		if found && err == nil {
			panic(fmt.Sprintf("deleted item %d found in store\n%s\n", m, idx.Root()))
		}
	}
	idx.mu.Unlock()
}

func (idx *testIndex) findRandomEntry(ctx context.Context, max int) error {
	n := int(rand.Int31n(int32(max)))
	want := newDetMockEntry(n)
	idx.mu.Lock()
	e, err := idx.Find(ctx, want.key)
	if errors.Is(err, pot.ErrNotFound) {
		err = nil
		if _, found := idx.deleted[n]; !found {
			err = fmt.Errorf("item %d not found in store", n)
		}
	}
	idx.mu.Unlock()
	if err != nil {
		return err
	}
	if e == nil {
		return nil
	}
	got := e.(*mockEntry)
	if !eq(want, got) {
		return fmt.Errorf("mismatch. want %v, got %v", want, got)
	}
	return nil
}

func newDetMockEntry(n int) *mockEntry {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(n))
	hasher := sha256.New()
	if _, err := hasher.Write(buf); err != nil {
		panic(err.Error())
	}
	return &mockEntry{hasher.Sum(nil), int(n)}
}

func checkFound(t *testing.T, ctx context.Context, idx *indigo.Index, want *mockEntry) {
	t.Helper()
	e, err := idx.Find(ctx, want.Key())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := e.(*mockEntry)
	if !ok {
		_ = e.(*mockEntry)
		t.Fatalf("incorrect value")
	}
	if !eq(want, got) {
		t.Fatalf("mismatch. want %v, got %v", want, got)
	}
}

func checkNotFound(t *testing.T, ctx context.Context, idx *indigo.Index, want *mockEntry) {
	t.Helper()
	_, err := idx.Find(ctx, want.Key())
	if !errors.Is(err, pot.ErrNotFound) {
		t.Fatalf("incorrect error returned for %d. want %v, got %v", want.val, pot.ErrNotFound, err)
	}
}

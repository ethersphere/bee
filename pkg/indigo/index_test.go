package indigo_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"

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
	idx, err := indigo.New(t.TempDir(), "test", func() pot.Entry { return &mockEntry{} })
	if err != nil {
		t.Fatal(err)
	}
	// missing := newMockEntry(t)
	want := newMockEntry(t)
	want2 := newMockEntry(t)
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
		idx.Add(ctx, want2)
		checkFound(t, ctx, idx, want2)
		checkFound(t, ctx, idx, want)
	})
	t.Run("delete first item and not find it", func(t *testing.T) {
		idx.Delete(ctx, want.Key())
		checkFound(t, ctx, idx, want2)
		checkNotFound(t, ctx, idx, want)
	})
	t.Run("readd first item and find both", func(t *testing.T) {
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

func checkFound(t *testing.T, ctx context.Context, idx *indigo.Index, want *mockEntry) {
	t.Helper()
	e, err := idx.Find(ctx, want.Key())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := e.(*mockEntry)
	if !ok {
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
		t.Fatalf("incorrect error returned. want %v, got %v", pot.ErrNotFound, err)
	}
}

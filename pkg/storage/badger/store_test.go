package badger_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage/badger"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
)

func TestStore(t *testing.T) {
	t.Parallel()

	store, err := badger.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	storagetest.TestStore(t, store)
}

func BenchmarkStore(b *testing.B) {
	store, err := badger.NewStore(b.TempDir())
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })
	storagetest.BenchmarkStore(b, store)
}

func TestBatchedStore(t *testing.T) {
	t.Parallel()

	store, err := badger.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	storagetest.TestBatchedStore(t, store)
}

func BenchmarkBatchedStore(b *testing.B) {
	store, err := badger.NewStore(b.TempDir())
	if err != nil {
		b.Fatalf("create store failed: %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })
	storagetest.BenchmarkBatchedStore(b, store)
}

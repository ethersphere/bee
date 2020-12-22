package batchservice_test

import (
	"bytes"
	crand "crypto/rand"
	"errors"
	"io"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchservice"
	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
)

var testLog = logging.New(ioutil.Discard, 0)

func TestNewBatchService(t *testing.T) {
	t.Run("expect get error", func(t *testing.T) {
		store := mock.New()
		store.SetGetError(errors.New("could not get"))
		_, err := batchservice.NewBatchService(store, testLog)
		if err == nil {
			t.Fatal("expected get error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		testChainState := postagetesting.NewChainState()
		store := mock.New(
			mock.WithChainState(testChainState),
		)
		_, err := batchservice.NewBatchService(store, testLog)
		if err != nil {
			t.Fatalf("new batch service: %v", err)
		}
	})
}

func TestBatchServiceCreate(t *testing.T) {
	testBatch := newTestBatch(t)

	t.Run("expect error", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		store.SetPutError(errors.New("could not put"))
		if err := svc.Create(testBatch.ID, testBatch.Owner, testBatch.Value, testBatch.Depth); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		err := svc.Create(testBatch.ID, testBatch.Owner, testBatch.Value, testBatch.Depth)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		got, err := store.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batchstore get: %v", err)
		}

		if !bytes.Equal(testBatch.ID, got.ID) {
			t.Fatalf("failed storing batch with ID: want %v, got %v", testBatch.ID, got.ID)
		}
	})
}

func TestBatchServiceTopUp(t *testing.T) {
	tBatch := newTestBatch(t)
	tTopUpAmount := big.NewInt(10000000000000)

	t.Run("expect get error", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		store.SetGetError(errors.New("failed to get"))
		err := svc.TopUp(tBatch.ID, tTopUpAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		store := mock.New()
		putBatch(t, store, tBatch)

		store.SetPutError(errors.New("could not put"))
		svc := newTestBatchService(t, store)

		err := svc.TopUp(tBatch.ID, tTopUpAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		putBatch(t, store, tBatch)
		svc := newTestBatchService(t, store)

		want := tBatch.Value
		want.Add(want, tTopUpAmount)

		if err := svc.TopUp(tBatch.ID, tTopUpAmount); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := store.Get(tBatch.ID)
		if err != nil {
			t.Fatalf("batchstore get: %v", err)
		}

		if got.Value.Cmp(want) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, want)
		}
	})
}

func TestBatchServiceUpdateDepth(t *testing.T) {
	const tNewDepth = 30
	tBatch := newTestBatch(t)

	t.Run("expect get error", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		store.SetGetError(errors.New("could not get"))
		if err := svc.UpdateDepth(tBatch.ID, tNewDepth); err == nil {
			t.Fatal("expect get error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		store := mock.New()
		putBatch(t, store, tBatch)
		svc := newTestBatchService(t, store)

		store.SetPutError(errors.New("could not put"))
		if err := svc.UpdateDepth(tBatch.ID, tNewDepth); err == nil {
			t.Fatal("expected put error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		putBatch(t, store, tBatch)
		svc := newTestBatchService(t, store)

		if err := svc.UpdateDepth(tBatch.ID, tNewDepth); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := store.Get(tBatch.ID)
		if err != nil {
			t.Fatalf("batchstore get: %v", err)
		}

		if val.Depth != tNewDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", tNewDepth, val.Depth)
		}
	})
}

func TestBatchServiceUpdatePrice(t *testing.T) {
	tChainState := postagetesting.NewChainState()
	tChainState.Price = big.NewInt(100000)
	tNewPrice := big.NewInt(20000000)

	t.Run("expect put error", func(t *testing.T) {
		store := mock.New()
		putChainState(t, store, tChainState)
		svc := newTestBatchService(t, store)

		store.SetPutError(errors.New("could not put"))
		if err := svc.UpdatePrice(tNewPrice); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		putChainState(t, store, tChainState)
		svc := newTestBatchService(t, store)

		if err := svc.UpdatePrice(tNewPrice); err != nil {
			t.Fatalf("update price: %v", err)
		}

		cs, err := store.GetChainState()
		if err != nil {
			t.Fatalf("store get chain state: %v", err)
		}

		if cs.Price.Cmp(tNewPrice) != 0 {
			t.Fatalf("bad price: want %v, got %v", cs.Price, tNewPrice)
		}
	})
}

func newTestBatchService(t *testing.T, store postage.BatchStorer) postage.EventUpdater {
	t.Helper()

	svc, err := batchservice.NewBatchService(store, testLog)
	if err != nil {
		t.Fatalf("new batch service: %v", err)
	}

	return svc
}

func newTestBatch(t *testing.T) *postage.Batch {
	t.Helper()

	b, err := postagetesting.NewBatch()
	if err != nil {
		t.Fatal("creating test batch")
	}

	return b
}

func putBatch(t *testing.T, store postage.BatchStorer, b *postage.Batch) {
	t.Helper()

	if err := store.Put(b); err != nil {
		t.Fatalf("store put batch: %v", err)
	}
}

func putChainState(t *testing.T, store postage.BatchStorer, cs *postage.ChainState) {
	t.Helper()

	if err := store.PutChainState(cs); err != nil {
		t.Fatalf("store put chain state: %v", err)
	}
}

func getRandomID(t *testing.T) []byte {
	t.Helper()

	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatalf("get random id: %v", err)
	}

	return id
}

func getRandomOwner(t *testing.T) []byte {
	t.Helper()

	owner := make([]byte, 20)
	_, err := io.ReadFull(crand.Reader, owner)
	if err != nil {
		t.Fatalf("get random owner: %v", owner)
	}

	return owner
}

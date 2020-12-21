package batchservice_test

import (
	"bytes"
	crand "crypto/rand"
	"errors"
	"io"
	"io/ioutil"
	"math/big"
	"math/rand"
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
	const testDepth = 15

	t.Run("expect error", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		id := getRandomID(t)
		owner := getRandomOwner(t)
		amount := (new(big.Int)).SetUint64(rand.Uint64())

		store.SetPutError(errors.New("could not put"))
		if err := svc.Create(id, owner, amount, testDepth); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		id := getRandomID(t)
		owner := getRandomOwner(t)
		amount := (new(big.Int)).SetUint64(rand.Uint64())
		err := svc.Create(id, owner, amount, testDepth)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		got, err := store.Get(id)
		if err != nil {
			t.Fatalf("batchstore get: %v", err)
		}

		if !bytes.Equal(got.ID, id) {
			t.Fatalf("batch ID not stored: want %v, got %v", id, got.ID)
		}
	})
}

func TestBatchServiceTopUp(t *testing.T) {
	testID := getRandomID(t)
	testAmount := big.NewInt(100000)

	t.Run("expect get error", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		id := getRandomID(t)
		store.SetGetError(errors.New("failed to get"))
		err := svc.TopUp(id, testAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		store := mock.New()
		if err := store.Put(&postage.Batch{
			ID:    testID,
			Value: big.NewInt(0),
		}); err != nil {
			t.Fatalf("store put: %v", err)
		}

		store.SetPutError(errors.New("could not put"))
		svc := newTestBatchService(t, store)

		err := svc.TopUp(testID, testAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		if err := store.Put(&postage.Batch{
			ID:    testID,
			Value: big.NewInt(0),
		}); err != nil {
			t.Fatalf("store put: %v", err)
		}
		svc := newTestBatchService(t, store)

		if err := svc.TopUp(testID, testAmount); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := store.Get(testID)
		if err != nil {
			t.Fatalf("batchstore get: %v", err)
		}

		// Value starts at zero, should be equal to test amount
		if got.Value.Cmp(testAmount) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, testAmount)
		}
	})
}

func TestBatchServiceUpdateDepth(t *testing.T) {
	const testDepth = 30
	testID := getRandomID(t)

	t.Run("expect get error", func(t *testing.T) {
		store := mock.New()
		svc := newTestBatchService(t, store)

		store.SetGetError(errors.New("could not get"))
		if err := svc.UpdateDepth(testID, testDepth); err == nil {
			t.Fatal("expect get error")
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		store := mock.New()
		if err := store.Put(&postage.Batch{
			ID: testID,
		}); err != nil {
			t.Fatalf("store put: %v", err)
		}
		svc := newTestBatchService(t, store)

		store.SetPutError(errors.New("could not put"))
		if err := svc.UpdateDepth(testID, testDepth); err == nil {
			t.Fatal("expected put error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		if err := store.Put(&postage.Batch{
			ID: testID,
		}); err != nil {
			t.Fatalf("store put: %v", err)
		}
		svc := newTestBatchService(t, store)

		if err := svc.UpdateDepth(testID, testDepth); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := store.Get(testID)
		if err != nil {
			t.Fatalf("batchstore get: %v", err)
		}

		if val.Depth != testDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", testDepth, val.Depth)
		}
	})
}

func TestBatchServiceUpdatePrice(t *testing.T) {
	testInitialPrice := big.NewInt(100000)
	testPrice := big.NewInt(20000000)

	t.Run("expect put error", func(t *testing.T) {
		store := mock.New()
		if err := store.PutChainState(&postage.ChainState{
			Price: testInitialPrice,
		}); err != nil {
			t.Fatalf("store put: %v", err)
		}
		svc := newTestBatchService(t, store)

		store.SetPutError(errors.New("could not put"))
		if err := svc.UpdatePrice(testPrice); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		store := mock.New()
		if err := store.PutChainState(&postage.ChainState{
			Price: testInitialPrice,
		}); err != nil {
			t.Fatalf("store put: %v", err)
		}
		svc := newTestBatchService(t, store)

		if err := svc.UpdatePrice(testPrice); err != nil {
			t.Fatalf("update price: %v", err)
		}

		cs, err := store.GetChainState()
		if err != nil {
			t.Fatalf("store get chain state: %v", err)
		}

		if cs.Price.Cmp(testPrice) != 0 {
			t.Fatalf("bad price: want %v, got %v", cs.Price, testPrice)
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

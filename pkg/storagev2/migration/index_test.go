package migration_test

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/migration"
)

func TestNewStepOnIndex(t *testing.T) {
	t.Parallel()

	t.Run("noop step", func(t *testing.T) {
		t.Parallel()

		const populateItemsCount = 100
		store := inmemstore.New()
		populateStore(t, store, populateItemsCount)

		stepFn := migration.NewStepOnIndex(
			storage.Query{
				Factory: newItemFactory,
			},
		)

		initialCount, err := store.Count(&item{})
		if err != nil {
			t.Fatalf("count should successed: %v", err)
		}
		if initialCount != populateItemsCount {
			t.Fatalf("have %d, want %d", initialCount, populateItemsCount)
		}

		if err := stepFn(store); err != nil {
			t.Fatalf("step migration should successed: %v", err)
		}

		afterStepCount, err := store.Count(&item{})
		if err != nil {
			t.Fatalf("count should successed: %v", err)
		}

		if afterStepCount != initialCount {
			t.Fatalf("step migration should not affect store")
		}
	})

	t.Run("delete items", func(t *testing.T) {
		t.Parallel()

		const populateItemsCount = 100
		store := inmemstore.New()
		populateStore(t, store, populateItemsCount)

		stepFn := migration.NewStepOnIndex(
			storage.Query{
				Factory:       newItemFactory,
				ItemAttribute: storage.QueryItem,
			},
			migration.WithItemDeleteFn(func(i storage.Item) bool {
				ii := i.(*item)
				return ii.val <= 9
			}),
		)

		if err := stepFn(store); err != nil {
			t.Fatalf("step migration should successed: %v", err)
		}

		afterStepCount, err := store.Count(&item{})
		if err != nil {
			t.Fatalf("count should successed: %v", err)
		}

		expectedCount := populateItemsCount - 10
		if afterStepCount != expectedCount {
			t.Fatalf("step migration should remove items; expected count: %d, have count %d", expectedCount, afterStepCount)
		}
	})

	t.Run("update items", func(t *testing.T) {
		t.Parallel()

		const populateItemsCount = 100
		const minVal = 50
		store := inmemstore.New()
		populateStore(t, store, populateItemsCount)

		stepFn := migration.NewStepOnIndex(
			storage.Query{
				Factory:       newItemFactory,
				ItemAttribute: storage.QueryItem,
			},
			// translate values from  [0 ... populateItemsCount) to [minVal ... populateItemsCount + minval)
			migration.WithItemUpdaterFn(func(i storage.Item) (storage.Item, bool) {
				ii := i.(*item)
				if ii.val < minVal {
					ii.val += populateItemsCount
					return ii, true
				}

				return nil, false
			}),
		)

		if err := stepFn(store); err != nil {
			t.Fatalf("step migration should successed: %v", err)
		}

		assertItemsInRange(t, store, minVal, populateItemsCount+minVal)
	})

	t.Run("delete and update items", func(t *testing.T) {
		t.Parallel()

		store := inmemstore.New()
		populateStore(t, store, 100)

		step := migration.NewStepOnIndex(
			storage.Query{
				Factory:       newItemFactory,
				ItemAttribute: storage.QueryItem,
			},
			// remove first 10 items
			migration.WithItemDeleteFn(func(i storage.Item) bool {
				ii := i.(*item)
				return ii.val < 10
			}),
			// translate values from [90-100) to [0-10) range
			migration.WithItemUpdaterFn(func(i storage.Item) (storage.Item, bool) {
				ii := i.(*item)
				if ii.val >= 90 {
					ii.val -= 90
					return ii, true
				}

				return nil, false
			}),
		)

		if err := step(store); err != nil {
			t.Fatalf("step migration should successed: %v", err)
		}

		assertItemsInRange(t, store, 0, 90)
	})
}

func TestOptions(t *testing.T) {
	t.Parallel()

	items := []*item{nil, {val: 1}, {val: 2}}

	t.Run("new options", func(t *testing.T) {
		t.Parallel()

		opts := migration.NewOptions()
		if opts == nil {
			t.Fatalf("options should not be nil")
		}

		deleteFn := opts.DeleteFn()
		if deleteFn == nil {
			t.Fatalf("options should have deleteFn specified")
		}

		updateFn := opts.UpdateFn()
		if updateFn == nil {
			t.Fatalf("options should have updateFn specified")
		}

		for _, i := range items {
			if deleteFn(i) != false {
				t.Fatalf("deleteFn should always return false")
			}

			if _, update := updateFn(i); update != false {
				t.Fatalf("updateFn should always return false")
			}
		}
	})

	t.Run("delete option apply", func(t *testing.T) {
		t.Parallel()

		itemC := make(chan storage.Item, 1)
		opts := migration.NewOptions()

		deleteFn := func(i storage.Item) bool {
			itemC <- i
			return false
		}
		opts.ApplyAll(migration.WithItemDeleteFn(deleteFn))

		for _, i := range items {
			opts.DeleteFn()(i)
			if !reflect.DeepEqual(i, <-itemC) {
				t.Fatalf("expecting applied deleteFn to be called")
			}
		}
	})

	t.Run("update option apply", func(t *testing.T) {
		t.Parallel()

		itemC := make(chan storage.Item, 1)
		opts := migration.NewOptions()

		updateFn := func(i storage.Item) (storage.Item, bool) {
			itemC <- i
			return i, false
		}
		opts.ApplyAll(migration.WithItemUpdaterFn(updateFn))

		for _, i := range items {
			opts.UpdateFn()(i)
			if !reflect.DeepEqual(i, <-itemC) {
				t.Fatalf("expecting applied updateFn to be called")
			}
		}
	})
}

func populateStore(t *testing.T, s storage.Store, count int) {
	t.Helper()

	for i := 0; i < count; i++ {
		item := &item{val: i}
		if err := s.Put(item); err != nil {
			t.Fatalf("populate store should successed: %v", err)
		}
	}
}

func assertItemsInRange(t *testing.T, s storage.Store, from, to int) {
	t.Helper()

	count, err := s.Count(&item{})
	if err != nil {
		t.Fatalf("count should successed: %v", err)
	}
	if count != to-from {
		t.Fatalf("have %d, want %d", count, (to - from))
	}

	err = s.Iterate(
		storage.Query{
			Factory:       newItemFactory,
			ItemAttribute: storage.QueryItem,
		},
		func(r storage.Result) (bool, error) {
			ii := r.Entry.(*item)
			if ii.val < from || ii.val >= to {
				return true, fmt.Errorf("item not in expected range: val %d", ii.val)
			}
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("populate store should successed: %v", err)
	}

}

type item struct {
	val int
}

func newItemFactory() storage.Item { return &item{} }

func (i *item) ID() string        { return strconv.Itoa(i.val) }
func (i *item) Namespace() string { return "migration-test" }

func (i *item) Marshal() ([]byte, error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(i.val))
	return buf, nil
}

func (i *item) Unmarshal(d []byte) error {
	i.val = int(binary.LittleEndian.Uint64(d))
	return nil
}

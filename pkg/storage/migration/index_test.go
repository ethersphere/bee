// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"fmt"
	"reflect"
	"testing"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
)

func TestNewStepOnIndex(t *testing.T) {
	t.Parallel()

	t.Run("noop step", func(t *testing.T) {
		t.Parallel()

		const populateItemsCount = 100
		store := inmemstore.New()
		populateStore(t, store, populateItemsCount)

		stepFn := migration.NewStepOnIndex(
			store,
			storage.Query{
				Factory: newObjFactory,
			},
		)

		initialCount, err := store.Count(&obj{})
		if err != nil {
			t.Fatalf("count should succeed: %v", err)
		}
		if initialCount != populateItemsCount {
			t.Fatalf("have %d, want %d", initialCount, populateItemsCount)
		}

		if err := stepFn(); err != nil {
			t.Fatalf("step migration should succeed: %v", err)
		}

		afterStepCount, err := store.Count(&obj{})
		if err != nil {
			t.Fatalf("count should succeed: %v", err)
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

		stepFn := migration.NewStepOnIndex(store,
			storage.Query{
				Factory:      newObjFactory,
				ItemProperty: storage.QueryItem,
			},
			migration.WithItemDeleteFn(func(i storage.Item) bool {
				o := i.(*obj)
				return o.val <= 9
			}),
			migration.WithOpPerBatch(3),
		)

		if err := stepFn(); err != nil {
			t.Fatalf("step migration should succeed: %v", err)
		}

		assertItemsInRange(t, store, 10, populateItemsCount)
	})

	t.Run("update items", func(t *testing.T) {
		t.Parallel()

		const populateItemsCount = 100
		const minVal = 50
		store := inmemstore.New()
		populateStore(t, store, populateItemsCount)

		stepFn := migration.NewStepOnIndex(store,
			storage.Query{
				Factory:      newObjFactory,
				ItemProperty: storage.QueryItem,
			},
			// translate values from  [0 ... populateItemsCount) to [minVal ... populateItemsCount + minval)
			migration.WithItemUpdaterFn(func(i storage.Item) (storage.Item, bool) {
				o := i.(*obj)
				if o.val < minVal {
					o.val += populateItemsCount
					return o, true
				}

				return nil, false
			}),
			migration.WithOpPerBatch(3),
		)

		if err := stepFn(); err != nil {
			t.Fatalf("step migration should succeed: %v", err)
		}

		assertItemsInRange(t, store, minVal, populateItemsCount+minVal)
	})

	t.Run("delete and update items", func(t *testing.T) {
		t.Parallel()

		const populateItemsCount = 100
		store := inmemstore.New()
		populateStore(t, store, populateItemsCount)

		step := migration.NewStepOnIndex(
			store,
			storage.Query{
				Factory:      newObjFactory,
				ItemProperty: storage.QueryItem,
			},
			// remove first 10 items
			migration.WithItemDeleteFn(func(i storage.Item) bool {
				o := i.(*obj)
				return o.val < 10
			}),
			// translate values from [90-100) to [0-10) range
			migration.WithItemUpdaterFn(func(i storage.Item) (storage.Item, bool) {
				o := i.(*obj)
				if o.val >= 90 {
					o.val -= 90
					return o, true
				}

				return nil, false
			}),
			migration.WithOpPerBatch(3),
		)

		if err := step(); err != nil {
			t.Fatalf("step migration should succeed: %v", err)
		}

		assertItemsInRange(t, store, 0, populateItemsCount-10)
	})

	t.Run("update with ID change", func(t *testing.T) {
		t.Parallel()

		const populateItemsCount = 100
		store := inmemstore.New()
		populateStore(t, store, populateItemsCount)

		step := migration.NewStepOnIndex(
			store,
			storage.Query{
				Factory:      newObjFactory,
				ItemProperty: storage.QueryItem,
			},
			migration.WithItemUpdaterFn(func(i storage.Item) (storage.Item, bool) {
				o := i.(*obj)
				o.id += 1
				return o, true
			}),
			migration.WithOpPerBatch(3),
		)

		if err := step(); err == nil {
			t.Fatalf("step migration should fail")
		}

		assertItemsInRange(t, store, 0, populateItemsCount)
	})
}

func TestStepIndex_BatchSize(t *testing.T) {
	t.Parallel()

	const populateItemsCount = 128
	for i := 1; i <= 2*populateItemsCount; i <<= 1 {
		t.Run(fmt.Sprintf("callback called once per item with batch size: %d", i), func(t *testing.T) {
			t.Parallel()

			store := inmemstore.New()
			populateStore(t, store, populateItemsCount)

			deleteItemCallMap := make(map[int]struct{})
			updateItemCallMap := make(map[int]struct{})

			stepFn := migration.NewStepOnIndex(
				store,
				storage.Query{
					Factory:      newObjFactory,
					ItemProperty: storage.QueryItem,
				},
				migration.WithItemDeleteFn(func(i storage.Item) bool {
					o := i.(*obj)
					if _, ok := deleteItemCallMap[o.id]; ok {
						t.Fatalf("delete should be called once")
					}
					deleteItemCallMap[o.id] = struct{}{}

					return o.id < 10
				}),
				migration.WithItemUpdaterFn(func(i storage.Item) (storage.Item, bool) {
					o := i.(*obj)
					if _, ok := updateItemCallMap[o.id]; ok {
						t.Fatalf("update should be called once")
					}
					updateItemCallMap[o.id] = struct{}{}

					return o, true
				}),
				migration.WithOpPerBatch(i),
			)

			if err := stepFn(); err != nil {
				t.Fatalf("step migration should succeed: %v", err)
			}

			opsExpected := (2 * populateItemsCount) - 10
			opsGot := len(updateItemCallMap) + len(deleteItemCallMap)
			if opsExpected != opsGot {
				t.Fatalf("updated and deleted items should add up to total: got %d, want %d", opsGot, opsExpected)
			}
		})
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()

	items := []*obj{nil, {id: 1}, {id: 2}}

	t.Run("new options", func(t *testing.T) {
		t.Parallel()

		opts := migration.DefaultOptions()
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

		if opts.OpPerBatch() <= 10 {
			t.Fatalf("default opPerBatch value is to small")
		}
	})

	t.Run("delete option apply", func(t *testing.T) {
		t.Parallel()

		itemC := make(chan storage.Item, 1)
		opts := migration.DefaultOptions()

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
		opts := migration.DefaultOptions()

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

	t.Run("opPerBatch option apply", func(t *testing.T) {
		t.Parallel()

		const opPerBetch = 3
		opts := migration.DefaultOptions()
		opts.ApplyAll(migration.WithOpPerBatch(opPerBetch))
		if opts.OpPerBatch() != opPerBetch {
			t.Fatalf("have %d, want %d", opts.OpPerBatch(), opPerBetch)
		}
	})
}

func populateStore(t *testing.T, s storage.Store, count int) {
	t.Helper()

	for i := 0; i < count; i++ {
		item := &obj{id: i, val: i}
		if err := s.Put(item); err != nil {
			t.Fatalf("populate store should succeed: %v", err)
		}
	}
}

func assertItemsInRange(t *testing.T, s storage.Store, from, to int) {
	t.Helper()

	count, err := s.Count(&obj{})
	if err != nil {
		t.Fatalf("count should succeed: %v", err)
	}
	if count != to-from {
		t.Fatalf("have %d, want %d", count, (to - from))
	}

	err = s.Iterate(
		storage.Query{
			Factory:      newObjFactory,
			ItemProperty: storage.QueryItem,
		},
		func(r storage.Result) (bool, error) {
			o := r.Entry.(*obj)
			if o.val < from || o.val >= to {
				return true, fmt.Errorf("item not in expected range: val %d", o.val)
			}
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("populate store should succeed: %v", err)
	}
}

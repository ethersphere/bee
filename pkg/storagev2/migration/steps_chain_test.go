// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/migration"
)

func TestNewStepsChain(t *testing.T) {
	t.Parallel()

	const populateItemsCount = 100
	store := inmemstore.New()
	populateStore(t, store, populateItemsCount)

	stepsFn := make([]migration.StepFn, 0)

	// Create 10 step functions where each would remove single element, having value [0-10)
	for i := 0; i < 10; i++ {
		valForRemoval := i
		var stepFn migration.StepFn

		// We create two types of step functions, each should have equivalent
		// behavior where each should remove only one element from store
		if i%2 == 0 {
			stepFn = migration.NewStepOnIndex(
				storage.Query{
					Factory:       newObjFactory,
					ItemAttribute: storage.QueryItem,
				},
				migration.WithItemDeleteFn(func(i storage.Item) bool {
					o := i.(*obj)
					return o.id == valForRemoval
				}),
			)
		} else {
			stepFn = func(s storage.Store) error {
				return s.Delete(&obj{id: valForRemoval})
			}
		}

		stepsFn = append(stepsFn, stepFn)
	}

	stepFn := migration.NewStepsChain(stepsFn...)
	if err := stepFn(store); err != nil {
		t.Fatalf("step migration should successed: %v", err)
	}

	afterStepCount, err := store.Count(&obj{})
	if err != nil {
		t.Fatalf("count should successed: %v", err)
	}

	expectedCount := populateItemsCount - 10
	if afterStepCount != expectedCount {
		t.Fatalf("step migration should remove items; expected count: %d, have count %d", expectedCount, afterStepCount)
	}
}

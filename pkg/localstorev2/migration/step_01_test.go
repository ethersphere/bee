// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	localmigration "github.com/ethersphere/bee/pkg/localstorev2/migration"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
)

func Test_Step_01(t *testing.T) {
	t.Parallel()

	stepFn := localmigration.Step_01
	dummyObj := localmigration.NewDummyObj(1635)

	// Test case when there is no dummyObj in store.
	// Asserts that stepFn executes without error and
	// that store will contain this object after step is executed.
	t.Run("no dummyObj", func(t *testing.T) {
		t.Parallel()

		store := inmemstore.New()

		assert.NoError(t, stepFn(store))

		has, err := store.Has(dummyObj)
		assert.NoError(t, err)
		assert.True(t, has)
	})

	// Test case when there is dummyObj already in store.
	// Asserts that stepFn executes without error and
	// that store will contain this object after step is executed.
	t.Run("with dummyObj", func(t *testing.T) {
		t.Parallel()

		store := inmemstore.New()
		assert.NoError(t, store.Put(dummyObj))

		assert.NoError(t, stepFn(store))

		has, err := store.Has(dummyObj)
		assert.NoError(t, err)
		assert.True(t, has)
	})
}

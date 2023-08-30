// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ethersphere/bee/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/pkg/storage/migration"
	localmigration "github.com/ethersphere/bee/pkg/storer/migration"
)

func TestAllSteps(t *testing.T) {
	t.Parallel()

	chStore := inmemchunkstore.New()

	assert.NotEmpty(t, localmigration.AllSteps("", 0, chStore))

	t.Run("version numbers", func(t *testing.T) {
		t.Parallel()

		err := migration.ValidateVersions(localmigration.AllSteps("", 0, chStore))
		assert.NoError(t, err)
	})

	t.Run("zero store migration", func(t *testing.T) {
		t.Parallel()

		store := inmemstore.New()

		err := migration.Migrate(store, localmigration.AllSteps("", 4, chStore))
		assert.NoError(t, err)
	})
}

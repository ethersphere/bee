// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	localmigration "github.com/ethersphere/bee/pkg/localstorev2/migration"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/migration"
)

func TestAllSteps(t *testing.T) {
	t.Parallel()

	assert.NotEmpty(t, localmigration.AllSteps())

	t.Run("version numbers", func(t *testing.T) {
		t.Parallel()

		err := migration.ValidateVersions(localmigration.AllSteps())
		assert.NoError(t, err)
	})

	t.Run("zero store migration", func(t *testing.T) {
		t.Parallel()

		store := inmemstore.New()

		err := migration.Migrate(store, localmigration.AllSteps())
		assert.NoError(t, err)
	})
}

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
	store := inmemstore.New()

	assert.NoError(t, stepFn(store))
}
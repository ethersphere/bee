// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func (db *DB) IteratePinCollection(root swarm.Address, iterateFn func(swarm.Address) (bool, error)) error {
	return pinstore.IterateCollection(db.storage.IndexStore(), root, iterateFn)
}

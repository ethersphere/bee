// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import storage "github.com/ethersphere/bee/pkg/storagev2"

func (db *DB) Repo() *storage.Repository {
	return db.repo
}

func ReplaceSharkyShardLimit(val int) {
	sharkyNoOfShards = val
}

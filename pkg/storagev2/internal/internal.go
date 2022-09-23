// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"

	"github.com/ethersphere/bee/pkg/storagev2"
)

// Storage groups the storage.Store and storage.ChunkStore interfaces with context..
type Storage interface {
	Ctx() context.Context
	Store() storage.Store
	ChunkStore() storage.ChunkStore
}

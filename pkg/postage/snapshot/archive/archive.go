// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package archive provides a snapshot.SnapshotGetter backed by the postage batch
// snapshot blob embedded in the binary at build time. It is the single importer
// of the (large) embedded archive, keeping that payload out of pkg/postage/snapshot
// and its other consumers.
package archive

import (
	batcharchive "github.com/ethersphere/batch-archive"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot"
)

var _ snapshot.SnapshotGetter = Getter{}

// Getter sources the postage batch snapshot from the embedded archive blob.
type Getter struct{}

func (Getter) GetBatchSnapshot() []byte {
	return batcharchive.GetBatchSnapshot()
}

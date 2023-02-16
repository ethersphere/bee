// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
)

type LocationResult struct {
	Err      error
	Location sharky.Location
}

// IterateLocations iterates over entire retrieval index and plucks only sharky location.
func IterateLocations(ctx context.Context, st storage.Store) <-chan LocationResult {
	locationResultC := make(chan LocationResult, 128)

	go func() {
		defer close(locationResultC)

		err := st.Iterate(storage.Query{
			Factory: func() storage.Item { return new(retrievalIndexItem) },
		}, func(r storage.Result) (bool, error) {
			select {
			case <-ctx.Done():
				return true, ctx.Err()
			default:
			}

			entry := r.Entry.(*retrievalIndexItem)
			locationResultC <- LocationResult{Location: entry.Location}

			return false, nil
		})
		if err != nil {
			locationResultC <- LocationResult{Err: fmt.Errorf("iterate retrieval index error: %w", err)}
		}
	}()

	return locationResultC
}

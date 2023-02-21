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
func IterateLocations(
	ctx context.Context,
	st storage.Store,
	locationResultC chan<- LocationResult,
) {
	go func() {
		defer close(locationResultC)

		err := st.Iterate(storage.Query{
			Factory: func() storage.Item { return new(retrievalIndexItem) },
		}, func(r storage.Result) (bool, error) {
			entry := r.Entry.(*retrievalIndexItem)
			result := LocationResult{Location: entry.Location}

			select {
			case <-ctx.Done():
				return true, ctx.Err()
			case locationResultC <- result:
			}

			return false, nil
		})
		if err != nil {
			result := LocationResult{Err: fmt.Errorf("iterate retrieval index error: %w", err)}

			select {
			case <-ctx.Done():
			case locationResultC <- result:
			}
		}
	}()
}

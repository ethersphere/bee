// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
)

type LocationResult struct {
	Err      error
	Location sharky.Location
}

type IterateResult struct {
	Err  error
	Item *RetrievalIndexItem
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
			Factory: func() storage.Item { return new(RetrievalIndexItem) },
		}, func(r storage.Result) (bool, error) {
			entry := r.Entry.(*RetrievalIndexItem)
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

// Iterate iterates over entire retrieval index with a call back.
func Iterate(st storage.Store, callBackFunc func(*RetrievalIndexItem) error) error {
	return st.Iterate(storage.Query{
		Factory: func() storage.Item { return new(RetrievalIndexItem) },
	}, func(r storage.Result) (bool, error) {
		entry := r.Entry.(*RetrievalIndexItem)
		return false, callBackFunc(entry)
	})
}

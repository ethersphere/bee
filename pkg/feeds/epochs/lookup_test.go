// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package epochs_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/feeds/epochs"
	feedstesting "github.com/ethersphere/bee/v2/pkg/feeds/testing"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
)

func TestFinder_FLAKY(t *testing.T) {
	t.Parallel()

	testf := func(t *testing.T, finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {
		t.Helper()

		t.Run("basic", func(t *testing.T) {
			feedstesting.TestFinderBasic(t, finderf, updaterf)
		})
		i := int64(0)
		nextf := func() (bool, int64) {
			defer func() { i++ }()
			return i < 50, i
		}
		t.Run("fixed", func(t *testing.T) {
			feedstesting.TestFinderFixIntervals(t, nextf, finderf, updaterf)
		})
		t.Run("random", func(t *testing.T) {
			feedstesting.TestFinderRandomIntervals(t, finderf, updaterf)
		})
	}
	t.Run("sync", func(t *testing.T) {
		t.Parallel()
		testf(t, epochs.NewFinder, epochs.NewUpdater)
	})
	t.Run("async", func(t *testing.T) {
		t.Parallel()
		testf(t, epochs.NewAsyncFinder, epochs.NewUpdater)
	})
}

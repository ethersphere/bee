// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package epochs_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/feeds/epochs"
	feedstesting "github.com/ethersphere/bee/pkg/feeds/testing"
	"github.com/ethersphere/bee/pkg/storage"
)

func TestFinder(t *testing.T) {
	testf := func(t *testing.T, finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic string) (feeds.Updater, error)) {
		t.Run("basic", func(t *testing.T) {
			feedstesting.TestFinderBasic(t, finderf, updaterf)
		})
		t.Run("fixed", func(t *testing.T) {
			feedstesting.TestFinderFixIntervals(t, finderf, updaterf)
		})
		t.Run("random", func(t *testing.T) {
			feedstesting.TestFinderRandomIntervals(t, finderf, updaterf)
		})
	}
	t.Run("sync", func(t *testing.T) {
		testf(t, epochs.NewFinder, epochs.NewUpdater)
	})
	t.Run("async", func(t *testing.T) {
		testf(t, epochs.NewAsyncFinder, epochs.NewUpdater)
	})
}

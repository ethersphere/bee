// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sequence_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/feeds/sequence"
	feedstesting "github.com/ethersphere/bee/pkg/feeds/testing"
	"github.com/ethersphere/bee/pkg/storage"
)

func TestFinder(t *testing.T) {
	testf := func(t *testing.T, finderf func(storage.Getter, *feeds.Feed) feeds.Lookup, updaterf func(putter storage.Putter, signer crypto.Signer, topic []byte) (feeds.Updater, error)) {
		t.Run("basic", func(t *testing.T) {
			feedstesting.TestFinderBasic(t, finderf, updaterf)
		})
		i := 0
		nextf := func() (bool, int64) {
			i++
			return i == 40, int64(i)
		}
		t.Run("fixed", func(t *testing.T) {
			feedstesting.TestFinderFixIntervals(t, nextf, finderf, updaterf)
		})
		t.Run("random", func(t *testing.T) {
			feedstesting.TestFinderRandomIntervals(t, finderf, updaterf)
		})
	}
	t.Run("sync", func(t *testing.T) {
		testf(t, sequence.NewFinder, sequence.NewUpdater)
	})
	t.Run("async", func(t *testing.T) {
		testf(t, sequence.NewAsyncFinder, sequence.NewUpdater)
	})
}

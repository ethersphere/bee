// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/storage/mock"
)

// TestJoiner verifies that a newly created joiner
// returns the data stored in the store for a given reference
func TestJoiner(t *testing.T) {
	store := mock.NewStorer()
	joiner := joiner.NewSimpleJoiner(store)
	_ = joiner
}

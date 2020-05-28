// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/file/entry"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestEntry(t *testing.T) {
	_ = entry.New(swarm.ZeroAddress)
}

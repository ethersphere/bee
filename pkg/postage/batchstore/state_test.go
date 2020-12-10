// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/postage/batchstore"
)

func TestStateMarshalling(t *testing.T) {
	state := &batchstore.State{
		block: 0,
	}

}

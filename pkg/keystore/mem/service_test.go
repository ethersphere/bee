// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mem_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/keystore/mem"
	"github.com/ethersphere/bee/v2/pkg/keystore/test"
)

func TestService(t *testing.T) {
	t.Parallel()

	test.Service(t, mem.New())
}

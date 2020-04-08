// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmem_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/addressbook/inmem"
	"github.com/ethersphere/bee/pkg/addressbook/test"
)

func TestInMem(t *testing.T) {
	test.Run(t, func(t *testing.T) (addressbook.GetPutter, func()) {
		return inmem.New(), func() {}
	})
}

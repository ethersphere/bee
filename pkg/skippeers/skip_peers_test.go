// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/skippeers"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestAddOverdraft(t *testing.T) {
	var (
		p1 = swarm.NewAddress([]byte("0xab"))
		p2 = swarm.NewAddress([]byte("0xbc"))
	)

	sp := new(skippeers.List)
	sp.Add(p1)

	t.Run("duplicate entries are ignored", func(t *testing.T) {
		sp.Add(p1)
		if len(sp.All()) != 1 {
			t.Errorf("expected len: %d, got %d", 1, len(sp.All()))
		}
	})

	t.Run("add peer", func(t *testing.T) {
		sp.Add(p2)
		if len(sp.All()) != 2 {
			t.Errorf("expected len: %d, got %d", 2, len(sp.All()))
		}
	})

	t.Run("add overdraft removes from addresses", func(t *testing.T) {
		sp.AddOverdraft(p2)

		if len(sp.All()) != 2 {
			t.Errorf("expected len: %d, got %d", 2, len(sp.All()))
		}
	})
}

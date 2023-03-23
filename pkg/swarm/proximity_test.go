// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package swarm_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/swarm"
)

// TestProximity validates Proximity function with explicit
// values in a table-driven test. It is highly dependant on
// MaxPO constant and it validates cases up to MaxPO=32.
func TestProximity(t *testing.T) {
	t.Parallel()

	base := []byte{0b00000000, 0b00000000, 0b00000000, 0b00000000}
	for _, tc := range []struct {
		addr []byte
		po   uint8
	}{
		{
			addr: base,
			po:   swarm.MaxPO,
		},
		{
			addr: []byte{0b10000000, 0b00000000, 0b00000000, 0b00000000},
			po:   0,
		},
		{
			addr: []byte{0b01000000, 0b00000000, 0b00000000, 0b00000000},
			po:   1,
		},
		{
			addr: []byte{0b00100000, 0b00000000, 0b00000000, 0b00000000},
			po:   2,
		},
		{
			addr: []byte{0b00010000, 0b00000000, 0b00000000, 0b00000000},
			po:   3,
		},
		{
			addr: []byte{0b00001000, 0b00000000, 0b00000000, 0b00000000},
			po:   4,
		},
		{
			addr: []byte{0b00000100, 0b00000000, 0b00000000, 0b00000000},
			po:   5,
		},
		{
			addr: []byte{0b00000010, 0b00000000, 0b00000000, 0b00000000},
			po:   6,
		},
		{
			addr: []byte{0b00000001, 0b00000000, 0b00000000, 0b00000000},
			po:   7,
		},
		{
			addr: []byte{0b00000000, 0b10000000, 0b00000000, 0b00000000},
			po:   8,
		},
		{
			addr: []byte{0b00000000, 0b01000000, 0b00000000, 0b00000000},
			po:   9,
		},
		{
			addr: []byte{0b00000000, 0b00100000, 0b00000000, 0b00000000},
			po:   10,
		},
		{
			addr: []byte{0b00000000, 0b00010000, 0b00000000, 0b00000000},
			po:   11,
		},
		{
			addr: []byte{0b00000000, 0b00001000, 0b00000000, 0b00000000},
			po:   12,
		},
		{
			addr: []byte{0b00000000, 0b00000100, 0b00000000, 0b00000000},
			po:   13,
		},
		{
			addr: []byte{0b00000000, 0b00000010, 0b00000000, 0b00000000},
			po:   14,
		},
		{
			addr: []byte{0b00000000, 0b00000001, 0b00000000, 0b00000000},
			po:   15,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b10000000, 0b00000000},
			po:   16,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b01000000, 0b00000000},
			po:   17,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00100000, 0b00000000},
			po:   18,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00010000, 0b00000000},
			po:   19,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00001000, 0b00000000},
			po:   20,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000100, 0b00000000},
			po:   21,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000010, 0b00000000},
			po:   22,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000001, 0b00000000},
			po:   23,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b10000000},
			po:   24,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b01000000},
			po:   25,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b00100000},
			po:   26,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b00010000},
			po:   27,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b00001000},
			po:   28,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b00000100},
			po:   29,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b00000010},
			po:   30,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b00000001},
			po:   31,
		},
		{
			addr: nil,
			po:   31,
		},
		{
			addr: []byte{0b00000001},
			po:   7,
		},
		{
			addr: []byte{0b00000000},
			po:   31,
		},
		{
			addr: []byte{0b00000000, 0b00000000, 0b00000000, 0b00000000, 0b00000001},
			po:   31,
		},
	} {
		x, y := swarm.NewAddress(base), swarm.NewAddress(tc.addr)

		got := swarm.Proximity(x, y)
		if got != tc.po {
			t.Errorf("got %v bin, want %v", got, tc.po)
		}
		got = swarm.Proximity(y, x)
		if got != tc.po {
			t.Errorf("got %v bin, want %v (reverse arguments)", got, tc.po)
		}
	}
}

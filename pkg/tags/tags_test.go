// Copyright 2019 The go-ethereum Authors
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

package tags

import (
	"testing"
)

func TestAll(t *testing.T) {
	ts := NewTags()
	if _, err := ts.Create("1", 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Create("2", 1, false); err != nil {
		t.Fatal(err)
	}

	all := ts.All()

	if len(all) != 2 {
		t.Fatalf("expected length to be 2 got %d", len(all))
	}

	if n := all[0].TotalCounter(); n != 1 {
		t.Fatalf("expected tag 0 Total to be 1 got %d", n)
	}

	if n := all[1].TotalCounter(); n != 1 {
		t.Fatalf("expected tag 1 Total to be 1 got %d", n)
	}

	if _, err := ts.Create("3", 1, false); err != nil {
		t.Fatal(err)
	}
	all = ts.All()

	if len(all) != 3 {
		t.Fatalf("expected length to be 3 got %d", len(all))
	}
}

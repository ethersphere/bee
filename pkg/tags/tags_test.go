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
	"context"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
)

func TestAll(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	ts := NewTags(logger)
	if _, err := ts.Create(1); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Create(1); err != nil {
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

	if _, err := ts.Create(1); err != nil {
		t.Fatal(err)
	}
	all = ts.All()

	if len(all) != 3 {
		t.Fatalf("expected length to be 3 got %d", len(all))
	}
}

func TestListAll(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	ts1 := NewTags(logger)

	// create few tags
	for i := 0; i < 5; i++ {
		if _, err := ts1.Create(1); err != nil {
			t.Fatal(err)
		}
	}

	// tags are from sync.Map
	tagList1, err := ts1.ListAll(context.Background(), 0, 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(tagList1) != 5 {
		t.Fatalf("want %d tags but got %d", 5, len(tagList1))
	}
}

// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package testing

import (
	"testing"

	"github.com/ethersphere/bee/pkg/tags"
)

// CheckTag checks the first tag in the api struct to be in a certain state
func CheckTag(t *testing.T, tag *tags.Tag, split, stored, seen, sent, synced, total int64) {
	t.Helper()
	if tag == nil {
		t.Fatal("no tag found")
	}
	tSplit := tag.Get(tags.StateSplit)
	if tSplit != split {
		t.Fatalf("should have had split chunks, got %d want %d", tSplit, split)
	}

	tSeen := tag.Get(tags.StateSeen)
	if tSeen != seen {
		t.Fatalf("should have had seen chunks, got %d want %d", tSeen, seen)
	}

	tStored := tag.Get(tags.StateStored)
	if tStored != stored {
		t.Fatalf("mismatch stored chunks, got %d want %d", tStored, stored)
	}

	tSent := tag.Get(tags.StateSent)
	if tStored != stored {
		t.Fatalf("mismatch sent chunks, got %d want %d", tSent, sent)
	}

	tSynced := tag.Get(tags.StateSynced)
	if tSynced != synced {
		t.Fatalf("mismatch synced chunks, got %d want %d", tSynced, synced)
	}

	tTotal := tag.TotalCounter()
	if tTotal != total {
		t.Fatalf("mismatch total chunks, got %d want %d", tTotal, total)
	}
}

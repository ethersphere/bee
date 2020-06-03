// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

// TestEntrySerialize verifies integrity of serialization.
func TestEntrySerialize(t *testing.T) {
	referenceAddress := test.RandomAddress()
	metadataAddress := test.RandomAddress()
	e := entry.New(referenceAddress, metadataAddress)
	entrySerialized, err := e.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	entryRecovered := &entry.Entry{}
	err = entryRecovered.UnmarshalBinary(entrySerialized)
	if err != nil {
		t.Fatal(err)
	}

	if !referenceAddress.Equal(entryRecovered.Reference()) {
		t.Fatalf("expected reference %s, got %s", referenceAddress, entryRecovered.Reference())
	}

	metadataAddressRecovered := entryRecovered.Metadata()
	if !metadataAddress.Equal(metadataAddressRecovered) {
		t.Fatalf("expected metadata %s, got %s", metadataAddress, metadataAddressRecovered)
	}
}

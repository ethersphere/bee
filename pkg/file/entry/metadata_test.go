// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/file/entry"
)

func TestMetadataSerialize(t *testing.T) {
	m := entry.NewMetadata("foo.bin")
	m = m.WithMimeType("text/plain")

	metadataBytes, err := m.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	metadataRecovered := &entry.Metadata{}
	err = metadataRecovered.UnmarshalBinary(metadataBytes)
	if err != nil {
		t.Fatal(err)
	}

	if m.Filename != metadataRecovered.Filename {
		t.Fatalf("Deserialize mismatch, expected %v, got %v", m.Filename, metadataRecovered.Filename)
	}

	if m.MimeType != metadataRecovered.MimeType {
		t.Fatalf("Deserialize mismatch, expected %v, got %v", m.MimeType, metadataRecovered.MimeType)
	}
}

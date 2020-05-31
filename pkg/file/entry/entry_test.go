// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry_test

import (
//	"bytes"
//	"context"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/collection"
	"github.com/ethersphere/bee/pkg/file/entry"
//	"github.com/ethersphere/bee/pkg/storage/mock"
//	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

type readNoopCloser struct {
	io.Reader
}

func NewReadNoopCloser(reader io.Reader) io.ReadCloser {
	return &readNoopCloser {
		Reader: reader,
	}
}

func (t *readNoopCloser) Close() error {
	return nil
}

func TestEntry(t *testing.T) {
	_ = entry.New(swarm.ZeroAddress)
}

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

func TestEntrySerialize(t *testing.T) {
	referenceAddress := test.RandomAddress()
	metadataAddress := test.RandomAddress()
	e := entry.New(referenceAddress)
	e.SetMetadata(metadataAddress)
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

	metadataAddressRecovered := entryRecovered.Metadata(collection.FilenameMimeType)
	if !metadataAddress.Equal(metadataAddressRecovered) {
		t.Fatalf("expected metadata %s, got %s", metadataAddress, metadataAddressRecovered)
	}
}

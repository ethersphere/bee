// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry_test

import (
//	"bytes"
//	"context"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/file/entry"
//	"github.com/ethersphere/bee/pkg/storage/mock"
//	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/swarm"
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
//	store := mock.NewStorer()
//	s := splitter.NewSimpleSplitter(store)
//	data := []byte("foo")
//	buf := bytes.NewBuffer(data)
//	bufCloser := NewReadNoopCloser(buf)
//	addr, err := s.Split(context.Background(), bufCloser, int64(len(data)))
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	e := entry.New(addr)
}

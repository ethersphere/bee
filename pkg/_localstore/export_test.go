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

package localstore

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestExportImport constructs two databases, one to put and export
// chunks and another one to import and validate that all chunks are
// imported.
func TestExportImport(t *testing.T) {
	db1 := newTestDB(t, nil)

	var chunkCount = 100

	chunks := make(map[string][]byte, chunkCount)
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()

		_, err := db1.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		stamp, err := ch.Stamp().MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		chunks[ch.Address().String()] = append(stamp, ch.Data()...)
	}

	var buf bytes.Buffer

	c, err := db1.Export(&buf)
	if err != nil {
		t.Fatal(err)
	}
	wantChunksCount := int64(len(chunks))
	if c != wantChunksCount {
		t.Errorf("got export count %v, want %v", c, wantChunksCount)
	}

	db2 := newTestDB(t, nil)

	c, err = db2.Import(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if c != wantChunksCount {
		t.Errorf("got import count %v, want %v", c, wantChunksCount)
	}

	for a, want := range chunks {
		addr := swarm.MustParseHexAddress(a)
		ch, err := db2.Get(context.Background(), storage.ModeGetRequest, addr)
		if err != nil {
			t.Fatal(err)
		}
		stamp, err := ch.Stamp().MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		got := append(stamp, ch.Data()...)
		if !bytes.Equal(got, want) {
			t.Fatalf("chunk %s: got stamp+data %x, want %x", addr, got[:256], want[:256])
		}
	}
}

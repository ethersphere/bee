// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feeds

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	soctesting "github.com/ethersphere/bee/v2/pkg/soc/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
)

func TestGetWrappedChunk(t *testing.T) {
	storer := mockstorer.New()

	data := []byte("data")
	// new format (wraps chunk)
	ch := soctesting.GenerateMockSOC(t, data).Chunk()
	wch, err := GetWrappedChunk(context.Background(), storer.ChunkStore(), ch, false)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(wch.Data()[8:], data) {
		t.Fatal("data mismatch")
	}

	// old format
	err = storer.Put(context.Background(), wch)
	if err != nil {
		t.Fatal(err)
	}

	tt := []struct {
		name string
		addr []byte
	}{
		{
			name: "unencrypted",
			addr: wch.Address().Bytes(),
		},
		{
			name: "encrypted",
			addr: append(wch.Address().Bytes(), wch.Address().Bytes()...),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			timestamp := make([]byte, 8)
			binary.BigEndian.PutUint64(timestamp, 1)
			ch = soctesting.GenerateMockSOC(t, append(timestamp, tc.addr...)).Chunk()

			wch, err = GetWrappedChunk(context.Background(), storer.ChunkStore(), ch, true)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(wch.Data()[8:], data) {
				t.Fatal("data mismatch")
			}
		})
	}

	t.Run("returns feed legacy payload", func(t *testing.T) {
		timestamp := make([]byte, 8)
		binary.BigEndian.PutUint64(timestamp, 1)
		feedChData := append(timestamp, wch.Address().Bytes()...)
		ch = soctesting.GenerateMockSOC(t, feedChData).Chunk()

		wch, err = GetWrappedChunk(context.Background(), storer.ChunkStore(), ch, false)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(wch.Data()[8:], feedChData) {
			t.Fatal("data should be similar as old legacy feed payload format.")
		}
	})
}

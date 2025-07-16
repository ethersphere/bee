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

	// new format (wraps chunk)
	ch := soctesting.GenerateMockSOC(t, []byte("data")).Chunk()
	wch, err := GetWrappedChunk(context.Background(), storer.ChunkStore(), ch)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(wch.Data()[8:], []byte("data")) {
		t.Fatal("data mismatch")
	}

	// old format
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

			err = storer.Put(context.Background(), wch)
			if err != nil {
				t.Fatal(err)
			}

			wch, err = GetWrappedChunk(context.Background(), storer.ChunkStore(), ch)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(wch.Data()[8:], []byte("data")) {
				t.Fatal("data mismatch")
			}
		})
	}
}

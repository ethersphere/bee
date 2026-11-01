// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feeds_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/soc"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// socData builds a valid single-owner-chunk wire payload (id + signature +
// wrapped cac data) wrapping the given cac payload, matching what a peer would
// send as the chunk backing a feed update.
func socData(f *testing.F, payload []byte) []byte {
	f.Helper()

	priv, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(priv)

	ch, err := cac.New(payload)
	if err != nil {
		f.Fatal(err)
	}
	id := make([]byte, swarm.HashSize)
	signed, err := soc.New(id, ch).Sign(signer)
	if err != nil {
		f.Fatal(err)
	}
	return signed.Data()
}

// FuzzGetWrappedChunk drives the feeds entrypoint that parses peer-supplied
// chunk bytes: FromChunk -> soc.FromChunk (cursor slicing + ecdsa recovery over
// attacker id/signature) and the feed-specific legacy payload split
// (legacyPayload/isV1Length + cacData[16:] -> ref -> getter.Get). Must not panic.
func FuzzGetWrappedChunk(f *testing.F) {
	// valid new-format SOC
	f.Add(socData(f, []byte("payload")), false)
	// legacy unencrypted: wrapped cac Data() len == 48 (span8 + timestamp8 + 32B ref)
	f.Add(socData(f, make([]byte, 40)), true)
	// legacy encrypted: wrapped cac Data() len == 80 (span8 + timestamp8 + 64B ref)
	f.Add(socData(f, make([]byte, 72)), true)
	// degenerate inputs
	f.Add([]byte(nil), false)
	f.Add(make([]byte, swarm.SocMinChunkSize-1), false)

	getter := mockstorer.New().ChunkStore()

	f.Fuzz(func(t *testing.T, data []byte, legacy bool) {
		ch := swarm.NewChunk(swarm.NewAddress(make([]byte, swarm.HashSize)), data)
		wc, err := feeds.GetWrappedChunk(context.Background(), getter, ch, legacy)
		if err == nil && wc == nil {
			t.Fatal("GetWrappedChunk returned nil chunk with nil error")
		}
	})
}

// FuzzFeedsFromChunk isolates the SOC-unwrap trust boundary reached through the
// feeds entrypoint (FromChunk + IsV1Payload) on arbitrary chunk data. Must not
// panic; on success the unwrapped chunk carries at least its 8-byte span.
func FuzzFeedsFromChunk(f *testing.F) {
	f.Add(socData(f, []byte("payload")))
	f.Add(socData(f, make([]byte, 40)))
	f.Add([]byte(nil))
	f.Add(make([]byte, swarm.SocMinChunkSize-1))

	f.Fuzz(func(t *testing.T, data []byte) {
		ch := swarm.NewChunk(swarm.NewAddress(make([]byte, swarm.HashSize)), data)
		wc, err := feeds.FromChunk(ch)
		if err == nil {
			if wc == nil {
				t.Fatal("FromChunk returned nil chunk with nil error")
			}
			if len(wc.Data()) < swarm.SpanSize {
				t.Fatalf("unwrapped chunk data shorter than span size: %d", len(wc.Data()))
			}
		}
		_, _ = feeds.IsV1Payload(ch)
	})
}

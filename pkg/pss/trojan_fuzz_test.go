// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzUnwrap drives the raw trojan parser pss.Unwrap directly. It exercises
// extractPublicKey (chunkData[36], chunkData[40:72]), the hint slice
// (chunkData[:8]), the ciphertext slice (chunkData[72:]) and the
// decryptAndCheck length path (plaintext[:2], plaintext[2:32],
// plaintext[32:32+length]).
func FuzzUnwrap(f *testing.F) {
	key := crypto.Secp256k1PrivateKeyFromBytes(bytes.Repeat([]byte{1}, 32))

	seedTopic := pss.NewTopic("seed-topic")
	topics := []pss.Topic{
		seedTopic,
		pss.NewTopic("other-topic-1"),
		pss.NewTopic("other-topic-2"),
	}

	fixedAddr := make([]byte, swarm.HashSize)

	// (a) valid round-trip trojan seed so the fuzzer starts from a chunk that
	// actually decrypts and can then mutate the length prefix / ciphertext.
	chunk, err := pss.Wrap(context.Background(), seedTopic, []byte("payload"), &key.PublicKey, newTargets(4, 1))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(chunk.Data())

	// (b) boundary/short slices probing the unguarded slicing.
	f.Add([]byte(nil))
	f.Add(make([]byte, 8))
	f.Add(make([]byte, 40))
	f.Add(make([]byte, 72))
	f.Add(make([]byte, swarm.ChunkWithSpanSize))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Mirror the production precondition: the only real caller,
		// (*pss).TryUnwrap, guards len(Data()) >= ChunkWithSpanSize before
		// invoking Unwrap. Peer-controlled bytes shorter than a chunk never
		// reach the unguarded slicing in Unwrap, so feeding them here would be
		// a false positive rather than a peer-reachable defect.
		if len(data) < swarm.ChunkWithSpanSize {
			return
		}
		chunk := swarm.NewChunk(swarm.NewAddress(fixedAddr), data)
		_, msg, err := pss.Unwrap(context.Background(), key, chunk, topics)
		if err == nil && msg != nil {
			if len(msg) > pss.MaxPayloadSize {
				t.Fatalf("decoded message length %d exceeds MaxPayloadSize %d", len(msg), pss.MaxPayloadSize)
			}
		}
	})
}

// FuzzTryUnwrap drives the production entry pss.(*pss).TryUnwrap via the
// pss.Interface. It exercises the len(Data()) < ChunkWithSpanSize guard plus
// Unwrap and the goroutine handler-dispatch logic. It is fire-and-forget, so
// the only assertion is absence of panic.
func FuzzTryUnwrap(f *testing.F) {
	key := crypto.Secp256k1PrivateKeyFromBytes(bytes.Repeat([]byte{1}, 32))

	seedTopic := pss.NewTopic("seed-topic")

	p := pss.New(key, log.Noop)
	// Non-blocking handler: goroutines dispatched by TryUnwrap must complete so
	// goleak in TestMain stays happy.
	_ = p.Register(seedTopic, func(context.Context, []byte) {})

	fixedAddr := make([]byte, swarm.HashSize)

	chunk, err := pss.Wrap(context.Background(), seedTopic, []byte("payload"), &key.PublicKey, newTargets(4, 1))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(chunk.Data())

	f.Add([]byte(nil))
	f.Add(make([]byte, 8))
	f.Add(make([]byte, 40))
	f.Add(make([]byte, 72))
	f.Add(make([]byte, swarm.ChunkWithSpanSize))

	f.Fuzz(func(t *testing.T, data []byte) {
		p.TryUnwrap(swarm.NewChunk(swarm.NewAddress(fixedAddr), data))
	})
}

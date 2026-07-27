// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzDeserializeGranteeList fuzzes the persisted grantee-list blob parser
// (deserialize), which decodes a concatenation of 65-byte secp256k1 public
// keys loaded from a file.LoadSaver, i.e. peer-controlled / persisted bytes.
// It asserts the offset-driven slicing never panics on any length, and checks
// the success invariants that the number of parsed keys is bounded by the
// input length and that any parsed key re-marshals canonically (round-trip).
func FuzzDeserializeGranteeList(f *testing.F) {
	keys, err := generateKeyListFixture()
	if err != nil {
		f.Fatal(err)
	}
	valid, err := accesscontrol.Serialize(keys)
	if err != nil {
		f.Fatal(err)
	}

	// Only non-crashing seeds so the seed corpus stays green; -fuzz is expected
	// to discover the non-multiple-of-65 lengths that trigger the slice panic.
	f.Add(valid)
	f.Add([]byte{})
	f.Add(make([]byte, accesscontrol.PublicKeyLen))
	f.Add(make([]byte, 2*accesscontrol.PublicKeyLen))

	f.Fuzz(func(t *testing.T, data []byte) {
		res := accesscontrol.Deserialize(data)

		// Success invariant: parsed keys cannot exceed input capacity.
		if len(res) > len(data)/accesscontrol.PublicKeyLen {
			t.Fatalf("parsed %d keys from %d bytes (max %d)", len(res), len(data), len(data)/accesscontrol.PublicKeyLen)
		}

		// Round-trip: parsed keys must re-marshal and re-parse to the same count.
		if len(res) > 0 {
			b, err := accesscontrol.Serialize(res)
			if err != nil {
				t.Fatalf("serialize of parsed keys: %v", err)
			}
			again := accesscontrol.Deserialize(b)
			if len(again) != len(res) {
				t.Fatalf("round-trip key count mismatch: got %d, want %d", len(again), len(res))
			}
		}
	})
}

// FuzzNewGranteeListReference fuzzes the exported end-to-end load path
// NewGranteeListReference, which loads a stored blob through a file.LoadSaver
// and parses it with the same deserialize(). This reproduces the parse surface
// through the public API exactly as a node loading a grantee list would.
func FuzzNewGranteeListReference(f *testing.F) {
	keys, err := generateKeyListFixture()
	if err != nil {
		f.Fatal(err)
	}
	valid, err := accesscontrol.Serialize(keys)
	if err != nil {
		f.Fatal(err)
	}

	f.Add(valid)
	f.Add([]byte{})
	f.Add(make([]byte, accesscontrol.PublicKeyLen))
	f.Add(make([]byte, 2*accesscontrol.PublicKeyLen))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64*1024 {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fresh store per iteration to avoid shared-state races under -parallel.
		st := mockstorer.New()
		ls := loadsave.New(st.ChunkStore(), st.Cache(), requestPipelineFactory(ctx, st.Cache(), false, redundancy.NONE), redundancy.DefaultDownloadLevel)

		ref, err := ls.Save(ctx, data)
		if err != nil {
			return
		}

		gl, err := accesscontrol.NewGranteeListReference(ctx, ls, swarm.NewAddress(ref))
		if err != nil {
			return
		}
		if gl == nil {
			t.Fatal("nil grantee list with nil error")
		}
		if len(gl.Get()) > len(data)/accesscontrol.PublicKeyLen {
			t.Fatalf("parsed %d keys from %d bytes", len(gl.Get()), len(data))
		}
	})
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray_test

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/manifest/mantaray"
)

// testMarshalOutput01 and testMarshalOutput02 are the two known-valid serialized
// nodes copied verbatim from marshal_test.go (v0.1 and v0.2, the latter carrying a
// metadata fork). They live in the internal test package and are not importable
// from mantaray_test, so the hex literals are inlined here.
const (
	testMarshalOutput01 = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64950ac787fbce1061870e8d34e0a638bc7e812c7ca4ebd31d626a572ba47b06f6952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072102654f163f5f0fa0621d729566c74d10037c4d7bbb0407d1e2c64950fcd3072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64950f89d6640e3044f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64850ff9f642182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64b50fc98072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64a50ff99622182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64d"
	testMarshalOutput02 = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64905954fb18659339d0b25e0fb9723d3cd5d528fb3c8d495fd157bd7b7a210496952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072102654f163f5f0fa0621d729566c74d10037c4d7bbb0407d1e2c64940fcd3072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952e3872548ec012a6e123b60f9177017fb12e57732621d2c1ada267adbe8cc4350f89d6640e3044f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64850ff9f642182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64b50fc98072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64a50ff99622182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64d"
)

// FuzzNodeUnmarshalBinary fuzzes mantaray.(*Node).UnmarshalBinary, the real
// exported decoder for peer-controlled / persisted manifest node bytes
// (marshal.go). It XOR-decrypts everything after the 32-byte obfuscation key,
// verifies a 31-byte version hash, reads a 1-byte refBytesSize, slices out the
// entry, reads a 32-byte fork bitvector, and for each set bit parses a fork
// (nodeType, prefixLen, prefix, ref, and for metadata forks a 2-byte length
// prefix + JSON). This is a classic length/offset-driven parser, so the primary
// invariant is that it must never panic on any input: a malformed buffer must
// surface as an error (ErrTooShort / ErrInvalidManifest / ErrInvalidVersionHash),
// never an out-of-bounds slice panic.
func FuzzNodeUnmarshalBinary(f *testing.F) {
	// (a) the two known-valid serialized nodes (v0.1 and v0.2).
	for _, h := range []string{testMarshalOutput01, testMarshalOutput02} {
		if b, err := hex.DecodeString(h); err == nil {
			f.Add(b)
		}
	}

	// (b) a constructor-built seed exercising several fork paths. forks is
	// non-nil after New(), so the nil LoadSaver is never dereferenced.
	n := mantaray.New()
	ctx := context.Background()
	for _, p := range []string{"aaaaa", "cc", "d"} {
		_ = n.Add(ctx, []byte(p), make([]byte, 32), nil, nil)
	}
	if b, err := n.MarshalBinary(); err == nil {
		f.Add(b)
	}

	// (c) short / degenerate buffers.
	f.Add([]byte(nil))
	f.Add(make([]byte, 63)) // one byte short of the header
	f.Add(make([]byte, 64)) // valid-length header, zero version hash

	f.Fuzz(func(t *testing.T, data []byte) {
		n := mantaray.New()
		if err := n.UnmarshalBinary(data); err != nil {
			return
		}

		// On a successful decode the node must re-marshal without error
		// (forks is set, so no ErrInvalidInput; ref widths are <=255 so no
		// size overflow), and that output must decode again cleanly. We do
		// NOT assert a byte-for-byte round-trip: refBytesSize inference and
		// root nodeType deduction can legitimately alter the re-encoding.
		out, err := n.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal after successful unmarshal: %v", err)
		}
		if err := mantaray.New().UnmarshalBinary(out); err != nil {
			t.Fatalf("re-unmarshal of marshaled output: %v", err)
		}
	})
}

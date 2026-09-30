// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/multiformats/go-multiaddr"
)

// fuzzHooks mirrors the preMapHooks wired in api.go (decHex/decBase64url) plus a
// resolve identity hook, so parseFieldTags can dispatch every hook name used by
// the carrier structs below without wiring a full node.
var fuzzHooks = map[string]func(v string) (string, error){
	"decBase64url": func(v string) (string, error) {
		buf, err := base64.URLEncoding.DecodeString(v)
		return string(buf), err
	},
	"decHex": func(v string) (string, error) {
		buf, err := hex.DecodeString(v)
		return string(buf), err
	},
	"resolve": func(v string) (string, error) { return v, nil },
}

// FuzzMapStructureUploadHeaders drives the real unexported api.mapStructure over
// the shared Tier-3 upload-header parse path: the numeric/bool/hex/address/pointer
// converters in set() plus the decHex/decBase64url preMap hooks. The carrier
// struct replicates the /bytes,/bzz,/chunks upload header struct. mapStructure
// wraps set()/hook() panics in a top-level recover, so an in-package panic
// surfaces as a returned error; the invariant here is that the call always
// returns (no panic escapes the recover, no hang, no unbounded allocation).
func FuzzMapStructureUploadHeaders(f *testing.F) {
	// Valid seed built the production way (the values a real client would send).
	f.Add(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // batchID: 64 hex chars
		"123",   // tag
		"true",  // pin
		"false", // deferred
		"1",     // rlevel
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // addr: 64 hex chars
		hex.EncodeToString([]byte("anchor-bytes")),                         // anchor (decHex)
		base64.URLEncoding.EncodeToString([]byte("exp-bytes")),             // exp (decBase64url)
	)
	// Benign edges.
	f.Add("", "", "", "", "", "", "", "")
	f.Add("0x", "-1", "zzz", "2", "99999999999999999999", "0x", "zz", "!!!")
	f.Add("g", "0", "1", "0", "255", "not-hex", "", "=")

	f.Fuzz(func(t *testing.T, batchID, tag, pin, deferredV, rlevel, addr, anchor, exp string) {
		out := struct {
			BatchID        []byte            `map:"Swarm-Postage-Batch-Id"`
			SwarmTag       uint64            `map:"Swarm-Tag"`
			Pin            bool              `map:"Swarm-Pin"`
			Deferred       *bool             `map:"Swarm-Deferred-Upload"`
			Encrypt        bool              `map:"Swarm-Encrypt"`
			RLevel         *redundancy.Level `map:"Swarm-Redundancy-Level"`
			HistoryAddress swarm.Address     `map:"Swarm-Act-History-Address"`
			Anchor         string            `map:"anchor,decHex"`
			Exp            string            `map:"exp,decBase64url"`
		}{}

		input := map[string][]string{
			"Swarm-Postage-Batch-Id":    {batchID},
			"Swarm-Tag":                 {tag},
			"Swarm-Pin":                 {pin},
			"Swarm-Deferred-Upload":     {deferredV},
			"Swarm-Redundancy-Level":    {rlevel},
			"Swarm-Act-History-Address": {addr},
			"anchor":                    {anchor},
			"exp":                       {exp},
		}

		// Must return normally; a non-nil error is an acceptable outcome.
		err := mapStructure(input, &out, fuzzHooks)
		if err == nil && out.RLevel != nil {
			_ = *out.RLevel // dereference must not panic on a successful decode
		}
	})
}

// gA compressed secp256k1 generator point — a valid 33-byte public key that
// btcec.ParsePubKey accepts, used to seed the pss.ParseRecipient branch.
const validCompressedPubKeyHex = "0279be667ef9dccbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"

// FuzzMapStructureExoticParsers drives the same real api.mapStructure over the
// exotic parser branches in set(): pss.ParseRecipient (Tier-5 secp256k1 pubkey
// decode via hex + btcec.ParsePubKey), big.Int base-10 SetString, common.Address
// / common.Hash hex decode, swarm.ParseHexAddress, and multiaddr.NewMultiaddr.
// These reach external decoders with attacker-controlled input — exactly the
// length/offset-from-input bug class. The invariant is that mapStructure always
// returns (no panic escaping recover, no hang, no OOM); errors are acceptable.
func FuzzMapStructureExoticParsers(f *testing.F) {
	// Valid seed.
	f.Add(
		validCompressedPubKeyHex, // recipient
		"12345",                  // amount
		"0x0123456789abcdef0123456789abcdef01234567",                         // ethaddr (40 hex)
		"0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // hash (64 hex)
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",   // swarmaddr (64 hex)
		"/ip4/127.0.0.1/tcp/1634",                                            // maddr
	)
	// Benign edges.
	f.Add("", "", "", "", "", "")
	f.Add("0x", "-", "0x", "0x", "0x", "/ip4/")
	f.Add("04", "99999999999999999999999999999999", "g", "z", "not-hex", "/tcp/x")

	f.Fuzz(func(t *testing.T, recipient, amount, ethaddr, hash, swarmaddr, maddr string) {
		out := struct {
			Publisher *ecdsa.PublicKey    `map:"Swarm-Act-Publisher"`
			Amount    *big.Int            `map:"amount"`
			Owner     common.Address      `map:"owner"`
			Hash      common.Hash         `map:"hash"`
			Addr      swarm.Address       `map:"address"`
			MAddr     multiaddr.Multiaddr `map:"maddr"`
		}{}

		input := map[string][]string{
			"Swarm-Act-Publisher": {recipient},
			"amount":              {amount},
			"owner":               {ethaddr},
			"hash":                {hash},
			"address":             {swarmaddr},
			"maddr":               {maddr},
		}

		// Must return normally; a non-nil error is an acceptable outcome.
		_ = mapStructure(input, &out, fuzzHooks)
	})
}

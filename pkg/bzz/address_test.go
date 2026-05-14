// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/multiformats/go-multiaddr"
)

func TestBzzAddress(t *testing.T) {
	t.Parallel()

	node1ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x2").Bytes()

	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err := crypto.NewOverlayAddress(privateKey1.PublicKey, 3, nonce)
	if err != nil {
		t.Fatal(err)
	}
	signer1 := crypto.NewDefaultSigner(privateKey1)

	chequebook := common.HexToAddress("0xabc0000000000000000000000000000000000123")

	bzzAddress, err := bzz.NewAddress(signer1, []multiaddr.Multiaddr{node1ma}, overlay, 3, nonce, 1, chequebook)
	if err != nil {
		t.Fatal(err)
	}

	bzzAddress2, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), bzzAddress.Signature, nonce, 1, 3, chequebook.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if !bzzAddress.Equal(bzzAddress2) {
		t.Fatalf("got %s expected %s", bzzAddress2, bzzAddress)
	}

	bytes, err := bzzAddress.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var newbzz bzz.Address
	if err := newbzz.UnmarshalJSON(bytes); err != nil {
		t.Fatal(err)
	}

	if !newbzz.Equal(bzzAddress) {
		t.Fatalf("got %s expected %s", newbzz.String(), bzzAddress.String())
	}
}

// TestParseAddress_ChequebookIsSigned guarantees that the chequebook address
// is covered by the BzzAddress signature. A relaying peer that swaps the
// chequebook field on the wire must cause signature verification to fail.
func TestParseAddress_ChequebookIsSigned(t *testing.T) {
	t.Parallel()

	node1ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}
	nonce := common.HexToHash("0x2").Bytes()
	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, 3, nonce)
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)

	signedChequebook := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tamperedChequebook := common.HexToAddress("0x2222222222222222222222222222222222222222")

	addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{node1ma}, overlay, 3, nonce, 1, signedChequebook)
	if err != nil {
		t.Fatal(err)
	}

	// Swapping the chequebook between sign and parse must invalidate the record.
	if _, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), addr.Signature, nonce, 1, 3, tamperedChequebook.Bytes()); !errors.Is(err, bzz.ErrInvalidAddress) {
		t.Fatalf("ParseAddress with tampered chequebook: want ErrInvalidAddress, got %v", err)
	}
}

func TestParseAddress_RejectsNonCanonicalChequebookLength(t *testing.T) {
	t.Parallel()

	node1ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}
	nonce := common.HexToHash("0x2").Bytes()
	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, 3, nonce)
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)

	cb := common.BytesToAddress([]byte{0x01, 0x02})
	addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{node1ma}, overlay, 3, nonce, 1, cb)
	if err != nil {
		t.Fatal(err)
	}

	// Each wire payload normalises through common.BytesToAddress back to
	// cb, so without the length guard the signature would validate.
	cbBytes := cb.Bytes()
	short2 := []byte{0x01, 0x02}
	long21 := append([]byte{0xff}, cbBytes...)
	long32 := append(make([]byte, 12), cbBytes...)
	long48 := append(make([]byte, 28), cbBytes...)

	cases := []struct {
		name string
		wire []byte
	}{
		{"short 2 bytes", short2},
		{"long 21 bytes", long21},
		{"long 32 bytes", long32},
		{"long 48 bytes", long48},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), addr.Signature, nonce, 1, 3, tc.wire); !errors.Is(err, bzz.ErrInvalidAddress) {
				t.Fatalf("want ErrInvalidAddress for %d-byte chequebook wire, got %v", len(tc.wire), err)
			}
		})
	}

	// Sanity: canonical lengths (0 and 20) must still parse.
	emptyAddr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{node1ma}, overlay, 3, nonce, 1, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), emptyAddr.Signature, nonce, 1, 3, nil); err != nil {
		t.Fatalf("empty chequebook must parse, got %v", err)
	}
	if _, err := bzz.ParseAddress(node1ma.Bytes(), overlay.Bytes(), addr.Signature, nonce, 1, 3, cbBytes); err != nil {
		t.Fatalf("20-byte chequebook must parse, got %v", err)
	}
}

func TestAreUnderlaysEqual(t *testing.T) {
	// --- Test Data Initialization ---
	addr1 := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/8001")
	addr2 := mustNewMultiaddr(t, "/ip4/192.168.1.1/tcp/8002")
	addr3 := mustNewMultiaddr(t, "/ip6/::1/udp/9000")
	addr4 := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/8001") // Identical to addr1

	// --- Test Cases Definition ---
	testCases := []struct {
		name string
		a    []multiaddr.Multiaddr
		b    []multiaddr.Multiaddr
		want bool
	}{
		{
			name: "two nil slices",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "one nil and one empty slice",
			a:    nil,
			b:    []multiaddr.Multiaddr{},
			want: true,
		},
		{
			name: "one empty and one nil slice",
			a:    []multiaddr.Multiaddr{},
			b:    nil,
			want: true,
		},
		{
			name: "two empty slices",
			a:    []multiaddr.Multiaddr{},
			b:    []multiaddr.Multiaddr{},
			want: true,
		},
		{
			name: "equal slices with same order",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr2},
			want: true,
		},
		{
			name: "equal slices with different order",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr3},
			b:    []multiaddr.Multiaddr{addr3, addr1, addr2},
			want: true,
		},
		{
			name: "equal slices with identical (but not same instance) values",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr4, addr2},
			want: true,
		},
		{
			name: "slices with different lengths (a < b)",
			a:    []multiaddr.Multiaddr{addr1},
			b:    []multiaddr.Multiaddr{addr1, addr2},
			want: false,
		},
		{
			name: "slices with different lengths (b < a)",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1},
			want: false,
		},
		{
			name: "slices with same length but different elements",
			a:    []multiaddr.Multiaddr{addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr3},
			want: false,
		},
		{
			name: "one slice is nil",
			a:    []multiaddr.Multiaddr{addr1},
			b:    nil,
			want: false,
		},
		{
			name: "slices with duplicates, equal",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr1},
			b:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			want: true,
		},
		{
			name: "slices with duplicates, not equal",
			a:    []multiaddr.Multiaddr{addr1, addr2, addr3},
			b:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			want: false,
		},
		{
			name: "slices with different duplicates",
			a:    []multiaddr.Multiaddr{addr1, addr1, addr2},
			b:    []multiaddr.Multiaddr{addr1, addr2, addr2},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := bzz.AreUnderlaysEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("AreUnderlaysEqual() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewAddressInvalidNonce(t *testing.T) {
	t.Parallel()

	node1ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}
	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	goodNonce := common.HexToHash("0x2").Bytes()
	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, 1, goodNonce)
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)

	for _, badNonce := range [][]byte{nil, {}, make([]byte, bzz.NonceLength-1), make([]byte, bzz.NonceLength+1)} {
		_, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{node1ma}, overlay, 1, badNonce, time.Now().Unix(), common.Address{})
		if !errors.Is(err, bzz.ErrInvalidAddress) {
			t.Fatalf("expected ErrInvalidAddress for nonce len %d, got %v", len(badNonce), err)
		}
	}
}

func TestParseAddressInvalidNonce(t *testing.T) {
	t.Parallel()

	for _, badNonce := range [][]byte{nil, {}, make([]byte, bzz.NonceLength-1), make([]byte, bzz.NonceLength+1)} {
		_, err := bzz.ParseAddress([]byte("underlay"), []byte("overlay"), []byte("sig"), badNonce, time.Now().Unix(), 1, nil)
		if !errors.Is(err, bzz.ErrInvalidAddress) {
			t.Fatalf("expected ErrInvalidAddress for nonce len %d, got %v", len(badNonce), err)
		}
	}
}

// TestGenerateSignData verifies that generateSignData embeds every field and
// that the output changes when any single field changes.
func TestGenerateSignData(t *testing.T) {
	t.Parallel()

	underlay := []byte("underlay-data")
	overlay := swarm.RandAddress(t).Bytes()
	var networkID uint64 = 1
	nonce := make([]byte, bzz.NonceLength)
	var timestamp int64 = 1_000_000
	cb := common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()

	base := bzz.GenerateSignData(underlay, overlay, networkID, nonce, timestamp, cb)

	// Prefix must be present at the start of the payload.
	if !bytes.HasPrefix(base, []byte("bee-handshake-")) {
		t.Fatal("sign data missing bee-handshake- prefix")
	}

	// Determinism: same inputs → identical bytes.
	if second := bzz.GenerateSignData(underlay, overlay, networkID, nonce, timestamp, cb); !bytes.Equal(base, second) {
		t.Fatal("generateSignData is not deterministic")
	}

	// Each field must contribute: mutating it must change the output.
	networkIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(networkIDBytes, networkID+1)

	cases := []struct {
		name string
		got  []byte
	}{
		{"underlay", bzz.GenerateSignData([]byte("other-underlay"), overlay, networkID, nonce, timestamp, cb)},
		{"overlay", bzz.GenerateSignData(underlay, swarm.RandAddress(t).Bytes(), networkID, nonce, timestamp, cb)},
		{"networkID", bzz.GenerateSignData(underlay, overlay, networkID+1, nonce, timestamp, cb)},
		{"nonce", bzz.GenerateSignData(underlay, overlay, networkID, bytes.Repeat([]byte{0x01}, bzz.NonceLength), timestamp, cb)},
		{"timestamp", bzz.GenerateSignData(underlay, overlay, networkID, nonce, timestamp+1, cb)},
		{"chequebook", bzz.GenerateSignData(underlay, overlay, networkID, nonce, timestamp,
			common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes())},
	}
	for _, tc := range cases {
		if bytes.Equal(base, tc.got) {
			t.Errorf("changing %s did not change sign data", tc.name)
		}
	}
}

func TestAddressEqual(t *testing.T) {
	t.Parallel()

	addr1 := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")
	addr2 := mustNewMultiaddr(t, "/ip4/192.168.1.1/tcp/1635")
	cb1 := common.HexToAddress("0xabc0000000000000000000000000000000000123")
	cb2 := common.HexToAddress("0xdef0000000000000000000000000000000000456")

	base := func() *bzz.Address {
		return &bzz.Address{
			Overlay:           swarm.MustParseHexAddress("0001"),
			Underlays:         []multiaddr.Multiaddr{addr1},
			Signature:         []byte{0xde, 0xad},
			Nonce:             []byte{0xbe, 0xef},
			Timestamp:         42,
			ChequebookAddress: cb1,
		}
	}
	with := func(mut func(*bzz.Address)) *bzz.Address {
		a := base()
		mut(a)
		return a
	}

	tests := []struct {
		name string
		a, b *bzz.Address
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil, b non-nil", nil, base(), false},
		{"a non-nil, b nil", base(), nil, false},
		{"equal full addresses", base(), base(), true},
		{"different overlay", base(), with(func(a *bzz.Address) { a.Overlay = swarm.MustParseHexAddress("0002") }), false},
		{"different underlays", base(), with(func(a *bzz.Address) { a.Underlays = []multiaddr.Multiaddr{addr2} }), false},
		{"different signature", base(), with(func(a *bzz.Address) { a.Signature = []byte{0xff} }), false},
		{"different nonce", base(), with(func(a *bzz.Address) { a.Nonce = []byte{0xff} }), false},
		{"different timestamp", base(), with(func(a *bzz.Address) { a.Timestamp = 999 }), false},
		{"different chequebook", base(), with(func(a *bzz.Address) { a.ChequebookAddress = cb2 }), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("Equal() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAddressString(t *testing.T) {
	t.Parallel()

	a := &bzz.Address{
		Overlay:           swarm.MustParseHexAddress("0001"),
		Underlays:         []multiaddr.Multiaddr{mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/1634")},
		Signature:         []byte{0xde, 0xad},
		Nonce:             []byte{0xbe, 0xef},
		Timestamp:         42,
		ChequebookAddress: common.HexToAddress("0xabc0000000000000000000000000000000000123"),
	}

	got := a.String()

	for _, want := range []string{
		"127.0.0.1",
		"dead",
		"beef",
		"abc0000000000000000000000000000000000123",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}

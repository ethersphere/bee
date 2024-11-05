// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestAddress(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		hex     string
		want    swarm.Address
		wantErr error
	}{
		{
			name: "blank",
			hex:  "",
			want: swarm.ZeroAddress,
		},
		{
			name:    "odd",
			hex:     "0",
			wantErr: hex.ErrLength,
		},
		{
			name: "zero",
			hex:  "00",
			want: swarm.NewAddress([]byte{0}),
		},
		{
			name: "all zeroes",
			hex:  swarm.EmptyAddress.String(),
			want: swarm.EmptyAddress,
		},
		{
			name: "one",
			hex:  "01",
			want: swarm.NewAddress([]byte{1}),
		},
		{
			name: "arbitrary",
			hex:  "35a26b7bb6455cbabe7a0e05aafbd0b8b26feac843e3b9a649468d0ea37a12b2",
			want: swarm.NewAddress([]byte{0x35, 0xa2, 0x6b, 0x7b, 0xb6, 0x45, 0x5c, 0xba, 0xbe, 0x7a, 0xe, 0x5, 0xaa, 0xfb, 0xd0, 0xb8, 0xb2, 0x6f, 0xea, 0xc8, 0x43, 0xe3, 0xb9, 0xa6, 0x49, 0x46, 0x8d, 0xe, 0xa3, 0x7a, 0x12, 0xb2}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, err := swarm.ParseHexAddress(tc.hex)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got error %v, want %v", err, tc.wantErr)
			}
			if a.String() != tc.want.String() {
				t.Errorf("got address %#v, want %#v", a, tc.want)
			}
			if !a.Equal(tc.want) {
				t.Errorf("address %v not equal to %v", a, tc.want)
			}
			if a.IsZero() != tc.want.IsZero() {
				t.Errorf("got address as zero=%v, want zero=%v", a.IsZero(), tc.want.IsZero())
			}
			if a.IsEmpty() != tc.want.IsEmpty() {
				t.Errorf("got address as empty=%v, want empty=%v", a.IsEmpty(), tc.want.IsEmpty())
			}
			if a.IsValidLength() != tc.want.IsValidLength() {
				t.Errorf("got address as invalid_size=%v, want valid=%v", a.IsValidLength(), tc.want.IsValidLength())
			}
		})
	}
}

func TestAddress_jsonMarshalling(t *testing.T) {
	t.Parallel()

	a1 := swarm.MustParseHexAddress("24798dd5a470e927fa")

	b, err := json.Marshal(a1)
	if err != nil {
		t.Fatal(err)
	}

	var a2 swarm.Address
	if err := json.Unmarshal(b, &a2); err != nil {
		t.Fatal(err)
	}

	if !a1.Equal(a2) {
		t.Error("unmarshalled address is not equal to the original")
	}
}

func TestValidSize(t *testing.T) {
	t.Parallel()

	a1 := swarm.MustParseHexAddress("24798dd5a470e927fa")
	a2 := swarm.MustParseHexAddress("35a26b7bb6455cbabe7a0e05aafbd0b8b26feac843e3b9a649468d0ea37a12b2")

	if a1.IsValidLength() {
		t.Fatal("wanted invalid size")
	}

	if !a2.IsValidLength() {
		t.Fatal("wanted valid size")
	}
}

func TestAddress_MemberOf(t *testing.T) {
	t.Parallel()

	a1 := swarm.MustParseHexAddress("24798dd5a470e927fa")
	a2 := swarm.MustParseHexAddress("24798dd5a470e927fa")
	a3 := swarm.MustParseHexAddress("24798dd5a470e927fb")
	a4 := swarm.MustParseHexAddress("24798dd5a470e927fc")

	set1 := []swarm.Address{a2, a3}
	if !a1.MemberOf(set1) {
		t.Fatal("expected addr as member")
	}

	set2 := []swarm.Address{a3, a4}
	if a1.MemberOf(set2) {
		t.Fatal("expected addr not member")
	}
}

func TestAddress_Clone(t *testing.T) {
	t.Parallel()

	original := swarm.MustParseHexAddress("35a26b7bb6455cbabe7a0e05aafbd0b8b26feac843e3b9a649468d0ea37a12b2")
	clone := original.Clone()

	if !original.Equal(clone) {
		t.Fatal("original should be equal with clone")
	}

	// if we modify original address, addresses should not be equal
	original.Bytes()[0] = 1
	if original.Equal(clone) {
		t.Fatal("original should not be equal with clone")
	}
}

func TestCloser(t *testing.T) {
	t.Parallel()

	a := swarm.MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000")
	x := swarm.MustParseHexAddress("8200000000000000000000000000000000000000000000000000000000000000")
	y := swarm.MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000")

	if cmp, _ := x.Closer(a, y); !cmp {
		t.Fatal("x is closer")
	}
}

func TestParseBitStr(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		overlay swarm.Address
		bitStr  string
	}{
		{
			swarm.MustParseHexAddress("5c32a2fe3d217af8c943fa665ebcfbdf7ab9af0cf1b2a1c8e5fc163dad2f5c7b"),
			"010111000",
		},
		{
			swarm.MustParseHexAddress("eac0903e59ff1c1a5f1d7d218b33f819b199aa0f68a19fd5fa02b7f84982b55d"),
			"111010101",
		},
		{
			swarm.MustParseHexAddress("70143dd2863ae07edfe7c1bfee75daea06226f0678e1117337d274492226bfe0"),
			"011100000",
		},
	} {
		if addr, err := swarm.ParseBitStrAddress(tc.bitStr); err != nil {
			t.Fatal(err)
		} else if got := swarm.Proximity(addr.Bytes(), tc.overlay.Bytes()); got < uint8(len(tc.bitStr)) {
			t.Fatalf("got proximiy %d want %d", got, len(tc.bitStr))
		}
	}
}

func TestNeighborhood(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		overlay swarm.Address
		bitStr  string
	}{
		{
			swarm.MustParseHexAddress("5c32a2fe3d217af8c943fa665ebcfbdf7ab9af0cf1b2a1c8e5fc163dad2f5c7b"),
			"010111000",
		},
		{
			swarm.MustParseHexAddress("eac0903e59ff1c1a5f1d7d218b33f819b199aa0f68a19fd5fa02b7f84982b55d"),
			"111010101",
		},
		{
			swarm.MustParseHexAddress("70143dd2863ae07edfe7c1bfee75daea06226f0678e1117337d274492226bfe0"),
			"011100000",
		},
	} {

		n := swarm.NewNeighborhood(tc.overlay, uint8(len(tc.bitStr)))
		if n.Equal(swarm.NewNeighborhood(swarm.RandAddress(t), uint8(len(tc.bitStr)))) {
			t.Fatal("addresses not should match")
		}
		if !n.Equal(swarm.NewNeighborhood(tc.overlay, uint8(len(tc.bitStr)))) {
			t.Fatal("addresses should match")
		}
		if !bytes.Equal(n.Bytes(), tc.overlay.Bytes()) {
			t.Fatal("bytes should match")
		}
		if n.String() != tc.bitStr {
			t.Fatal("bit str should match")
		}
	}
}

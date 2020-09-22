// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestAddress(t *testing.T) {
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
		})
	}
}

func TestAddress_jsonMarshalling(t *testing.T) {
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

func TestValidators(t *testing.T) {
	cv := content.NewValidator()
	sv := soc.NewValidator()
	chunkValidator := swarm.NewChunkValidator(sv, cv)

	// test a content chunk validation
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)
	foo := "foo"
	fooLength := len(foo)
	fooBytes := make([]byte, 8+fooLength)
	binary.LittleEndian.PutUint64(fooBytes, uint64(fooLength))
	copy(fooBytes[8:], foo)
	ch := swarm.NewChunk(address, fooBytes)
	yes, cType := chunkValidator.Validate(ch)
	if !yes {
		t.Fatalf("data '%s' should have validated to hash '%s'", ch.Data(), ch.Address())
	}
	if cType != swarm.ContentChunk {
		t.Fatal("invalid chunk type")
	}

	// test a SOC chunk validation
	id := make([]byte, soc.IdSize)
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	sch, err := soc.NewChunk(id, ch, signer)
	if err != nil {
		t.Fatal(err)
	}

	yes, cType = chunkValidator.Validate(sch)
	if !yes {
		t.Fatalf("data '%s' should have validated to hash '%s'", ch.Data(), ch.Address())
	}
	if cType != swarm.SingleOwnerChunk {
		t.Fatal("invalid chunk type")
	}
}

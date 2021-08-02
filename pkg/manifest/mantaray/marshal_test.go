// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/rand"
	mrand "math/rand"
	"reflect"
	"testing"

	"golang.org/x/crypto/sha3"
)

const testMarshalOutput01 = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64950ac787fbce1061870e8d34e0a638bc7e812c7ca4ebd31d626a572ba47b06f6952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072102654f163f5f0fa0621d729566c74d10037c4d7bbb0407d1e2c64950fcd3072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64950f89d6640e3044f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64850ff9f642182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64b50fc98072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64a50ff99622182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64d"

const testMarshalOutput02 = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64905954fb18659339d0b25e0fb9723d3cd5d528fb3c8d495fd157bd7b7a210496952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072102654f163f5f0fa0621d729566c74d10037c4d7bbb0407d1e2c64940fcd3072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952e3872548ec012a6e123b60f9177017fb12e57732621d2c1ada267adbe8cc4350f89d6640e3044f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64850ff9f642182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64b50fc98072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64a50ff99622182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64d"

var testEntries = []NodeEntry{
	{
		Path: []byte("/"),
		Metadata: map[string]string{
			"index-document": "aaaaa",
		},
	},
	{
		Path: []byte("aaaaa"),
	},
	{
		Path: []byte("cc"),
	},
	{
		Path: []byte("d"),
	},
	{
		Path: []byte("ee"),
	},
}

type NodeEntry struct {
	Path     []byte
	Entry    []byte
	Metadata map[string]string
}

func init() {
	obfuscationKeyFn = mrand.Read
}

func TestVersion01(t *testing.T) {
	hasher := sha3.NewLegacyKeccak256()

	_, err := hasher.Write([]byte(version01String))
	if err != nil {
		t.Fatal(err)
	}
	sum := hasher.Sum(nil)

	sumHex := hex.EncodeToString(sum)

	if version01HashString != sumHex {
		t.Fatalf("expecting version hash '%s', got '%s'", version01String, sumHex)
	}
}

func TestVersion02(t *testing.T) {
	hasher := sha3.NewLegacyKeccak256()

	_, err := hasher.Write([]byte(version02String))
	if err != nil {
		t.Fatal(err)
	}
	sum := hasher.Sum(nil)

	sumHex := hex.EncodeToString(sum)

	if version02HashString != sumHex {
		t.Fatalf("expecting version hash '%s', got '%s'", version02String, sumHex)
	}
}

func TestUnmarshal01(t *testing.T) {
	input, _ := hex.DecodeString(testMarshalOutput01)
	n := &Node{}
	err := n.UnmarshalBinary(input)
	if err != nil {
		t.Fatalf("expected no error marshaling, got %v", err)
	}

	expEncrypted := testMarshalOutput01[128:192]
	// perform XOR decryption
	expEncryptedBytes, _ := hex.DecodeString(expEncrypted)
	expBytes := encryptDecrypt(expEncryptedBytes, n.obfuscationKey)
	exp := hex.EncodeToString(expBytes)

	if hex.EncodeToString(n.entry) != exp {
		t.Fatalf("expected %x, got %x", exp, n.entry)
	}
	if len(testEntries) != len(n.forks) {
		t.Fatalf("expected %d forks, got %d", len(testEntries), len(n.forks))
	}
	for _, entry := range testEntries {
		prefix := entry.Path
		f := n.forks[prefix[0]]
		if f == nil {
			t.Fatalf("expected to have  fork on byte %x", prefix[:1])
		}
		if !bytes.Equal(f.prefix, prefix) {
			t.Fatalf("expected prefix for byte %x to match %s, got %s", prefix[:1], prefix, f.prefix)
		}
	}
}

func TestUnmarshal02(t *testing.T) {
	input, _ := hex.DecodeString(testMarshalOutput02)
	n := &Node{}
	err := n.UnmarshalBinary(input)
	if err != nil {
		t.Fatalf("expected no error marshaling, got %v", err)
	}

	expEncrypted := testMarshalOutput02[128:192]
	// perform XOR decryption
	expEncryptedBytes, _ := hex.DecodeString(expEncrypted)
	expBytes := encryptDecrypt(expEncryptedBytes, n.obfuscationKey)
	exp := hex.EncodeToString(expBytes)

	if hex.EncodeToString(n.entry) != exp {
		t.Fatalf("expected %x, got %x", exp, n.entry)
	}
	if len(testEntries) != len(n.forks) {
		t.Fatalf("expected %d forks, got %d", len(testEntries), len(n.forks))
	}
	for _, entry := range testEntries {
		prefix := entry.Path
		f := n.forks[prefix[0]]
		if f == nil {
			t.Fatalf("expected to have  fork on byte %x", prefix[:1])
		}
		if !bytes.Equal(f.prefix, prefix) {
			t.Fatalf("expected prefix for byte %x to match %s, got %s", prefix[:1], prefix, f.prefix)
		}
		if len(entry.Metadata) > 0 {
			if !reflect.DeepEqual(entry.Metadata, f.metadata) {
				t.Fatalf("expected metadata for byte %x to match %s, got %s", prefix[:1], entry.Metadata, f.metadata)
			}
		}
	}
}

func TestMarshal(t *testing.T) {
	rand.Seed(1)
	ctx := context.Background()
	n := New()
	defer func(r func(*fork) []byte) { refBytes = r }(refBytes)
	i := uint8(0)
	refBytes = func(*fork) []byte {
		b := make([]byte, 32)
		b[31] = i
		i++
		return b
	}
	for i := 0; i < len(testEntries); i++ {
		c := testEntries[i].Path
		e := testEntries[i].Entry
		if len(e) == 0 {
			e = append(make([]byte, 32-len(c)), c...)
		}
		m := testEntries[i].Metadata
		err := n.Add(ctx, c, e, m, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	}
	b, err := n.MarshalBinary()
	if err != nil {
		t.Fatalf("expected no error marshaling, got %v", err)
	}
	exp, _ := hex.DecodeString(testMarshalOutput02)
	if !bytes.Equal(b, exp) {
		t.Fatalf("expected marshalled output to match %x, got %x", exp, b)
	}
	// n = &Node{}
	// err = n.UnmarshalBinary(b)
	// if err != nil {
	// 	t.Fatalf("expected no error unmarshaling, got %v", err)
	// }

	// for j := 0; j < len(testCases); j++ {
	// 	d := testCases[j]
	// 	m, err := n.Lookup(d, nil)
	// 	if err != nil {
	// 		t.Fatalf("expected no error, got %v", err)
	// 	}
	// 	if !bytes.Equal(m, d) {
	// 		t.Fatalf("expected value %x, got %x", d, m)
	// 	}
	// }
}

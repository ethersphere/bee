// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"golang.org/x/crypto/sha3"
)

const testMarshalOutput01 = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64950ac787fbce1061870e8d34e0a638bc7e812c7ca4ebd31d626a572ba47b06f6952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072102654f163f5f0fa0621d729566c74d10037c4d7bbb0407d1e2c64950fcd3072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64950f89d6640e3044f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64850ff9f642182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64b50fc98072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64a50ff99622182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64d"

const testMarshalOutput02 = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64905954fb18659339d0b25e0fb9723d3cd5d528fb3c8d495fd157bd7b7a210496952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072102654f163f5f0fa0621d729566c74d10037c4d7bbb0407d1e2c64940fcd3072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952e3872548ec012a6e123b60f9177017fb12e57732621d2c1ada267adbe8cc4350f89d6640e3044f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64850ff9f642182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64b50fc98072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64a50ff99622182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64952fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64d"

var obfuscationKey = []uint8{82, 253, 252, 7, 33, 130, 101, 79, 22, 63, 95, 15, 154, 98, 29, 114, 149, 102, 199, 77, 16, 3, 124, 77, 123, 187, 4, 7, 209, 226, 198, 73}

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

// nolint:gochecknoinits
func init() {
	SetObfuscationKeyFn(func(b []byte) (int, error) {
		copy(b, obfuscationKey)
		return 32, nil
	})
}

func TestVersion01(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

// nolint:paralleltest
func TestMarshal(t *testing.T) {
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

func Test_UnmarshalBinary(t *testing.T) {
	t.Parallel()

	decode := func(s string) []byte {
		t.Helper()
		data, err := hex.DecodeString(s)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return data
	}

	tests := []struct {
		name    string // name for test case
		data    []byte // node binary data
		wantErr error  // tells if UnmarshalBinary should return error
	}{
		{
			name:    "nil input bytes",
			data:    nil,
			wantErr: ErrTooShort,
		},
		{
			name:    "not enough bytes for header",
			data:    make([]byte, nodeHeaderSize-1),
			wantErr: ErrTooShort,
		},
		{
			name:    "invalid header - empty bytes",
			data:    make([]byte, nodeHeaderSize),
			wantErr: ErrInvalidVersionHash,
		},
		{ // fork with metadata where metadata size value is less than metadata content (size 89, want 93)
			name:    "invalid manifest size 89",
			data:    decode("00000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f200000000000000000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000016012f0000000000000000000000000000000000000000000000000000000000e87f95c3d081c4fede769b6c69e27b435e525cbd25c6715c607e7c531e32963900597b22776562736974652d696e6465782d646f63756d656e74223a2233356561656538316262363338303436393965633637316265323736326465626665346662643330636461646139303232393239646131613965366134366436227d0a"),
			wantErr: ErrInvalidManifest,
		},
		{ // fork with valid metadata (size is 93, want 93)
			name:    "valid manifest",
			data:    decode("00000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f200000000000000000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000016012f0000000000000000000000000000000000000000000000000000000000e87f95c3d081c4fede769b6c69e27b435e525cbd25c6715c607e7c531e329639005d7b22776562736974652d696e6465782d646f63756d656e74223a2233356561656538316262363338303436393965633637316265323736326465626665346662643330636461646139303232393239646131613965366134366436227d0a"),
			wantErr: nil,
		},
		{ // fork with metadata where metadata size value exceeds actual metadata size (size is 95, want 93)
			name:    "invalid manifest size 95",
			data:    decode("00000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f200000000000000000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000016012f0000000000000000000000000000000000000000000000000000000000e87f95c3d081c4fede769b6c69e27b435e525cbd25c6715c607e7c531e329639005f7b22776562736974652d696e6465782d646f63756d656e74223a2233356561656538316262363338303436393965633637316265323736326465626665346662643330636461646139303232393239646131613965366134366436227d0a"),
			wantErr: ErrInvalidManifest,
		},
		{ // fork with metadata where metadata size value exceeds actual metadata size (size is 96, want 93)
			name:    "invalid manifest size 96",
			data:    decode("00000000000000000000000000000000000000000000000000000000000000005768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f200000000000000000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000016012f0000000000000000000000000000000000000000000000000000000000e87f95c3d081c4fede769b6c69e27b435e525cbd25c6715c607e7c531e32963900607b22776562736974652d696e6465782d646f63756d656e74223a2233356561656538316262363338303436393965633637316265323736326465626665346662643330636461646139303232393239646131613965366134366436227d0a"),
			wantErr: ErrInvalidManifest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			n := New()
			haveErr := n.UnmarshalBinary(tc.data)

			if tc.wantErr != nil && !errors.Is(haveErr, tc.wantErr) {
				t.Fatalf("expected error marshaling, want %v got %v", tc.wantErr, haveErr)
			} else if tc.wantErr == nil && haveErr != nil {
				t.Fatalf("unexpected error marshaling, got %v", haveErr)
			}
		})
	}
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// expectedChecksums pins the SHA-256 of the SIMD .syso blobs that this package
// links in. The values must match the contents of the CHECKSUM file in this
// directory; both are checked against the on-disk files by TestSysoChecksums
// so any accidental rebuild or corrupted check-in is caught on CI.
//
// To refresh after a legitimate rebuild (see README.md):
//
//	sha256sum keccak_times4_linux_amd64.syso \
//	         keccak_times8_linux_amd64.syso > CHECKSUM
//
// then update the constants below to match.
var expectedChecksums = map[string]string{
	"keccak_times4_linux_amd64.syso": "398953aa6d446ef64210a94ddf1f807ad6b9137d913b66adf83bbd6f665d5fbd",
	"keccak_times8_linux_amd64.syso": "9e53e1b590f657b2c817b816126111dedc167b5b7543e50106dd4435fee48b3c",
}

func TestSysoChecksums(t *testing.T) {
	t.Run("matches hardcoded values", func(t *testing.T) {
		for name, want := range expectedChecksums {
			got, err := sha256File(name)
			if err != nil {
				t.Fatalf("hashing %s: %v", name, err)
			}
			if got != want {
				t.Errorf("syso digest drift for %s:\n  want %s\n   got %s\n"+
					"if this rebuild is intentional, update expectedChecksums and CHECKSUM (see pkg/keccak/README.md)",
					name, want, got)
			}
		}
	})

	t.Run("CHECKSUM file is in sync", func(t *testing.T) {
		// guards against a maintainer updating expectedChecksums but forgetting
		// to refresh CHECKSUM (or vice versa) — both views of the digest must agree.
		fileSums, err := readChecksumFile("CHECKSUM")
		if err != nil {
			t.Fatalf("reading CHECKSUM: %v", err)
		}
		if len(fileSums) != len(expectedChecksums) {
			t.Fatalf("CHECKSUM lists %d files, expectedChecksums lists %d", len(fileSums), len(expectedChecksums))
		}
		for name, want := range expectedChecksums {
			got, ok := fileSums[name]
			if !ok {
				t.Errorf("CHECKSUM is missing entry for %s", name)
				continue
			}
			if got != want {
				t.Errorf("CHECKSUM disagrees with expectedChecksums for %s:\n  expectedChecksums: %s\n  CHECKSUM:          %s",
					name, want, got)
			}
		}
	})
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// readChecksumFile parses the standard `sha256sum` output format:
//
//	<hex-digest>  <filename>
//
// Lines that are blank or start with '#' are ignored.
func readChecksumFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		out[fields[len(fields)-1]] = fields[0]
	}
	return out, scanner.Err()
}

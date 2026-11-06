// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/updatecheck"
)

var newerRelease = release{version: 2000, tags: []string{"2.9.0"}}

func TestSignedRelease(t *testing.T) {
	t.Parallel()

	reg := newRegistry()
	// The unsigned summary says something different, and it must not be read.
	reg.set("/info", response{body: info(t, release{version: 5000, tags: []string{"9.0.0"}}, true)})
	body := descriptor(t, newerRelease)
	reg.set("/release.json", response{body: body})
	// Entries for other keys, including junk, are ignored.
	reg.set("/release.sig", response{body: sigSet(t,
		sig(otherSigner, body), testSig{KeyID: "junk"}, testSig{}, sig(testSigner, body))})

	s, err := updatecheck.NewUnstarted(log.Noop, runnerOptions(reg))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := updatecheck.Result{Current: "2.8.0", Latest: "2.9.0", LatestVersion: 2000, Available: true}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if reg.count("/info") != 0 {
		t.Fatal("read the unsigned /info with a release key")
	}
}

// bee refuses exactly what bee-runner refuses. That way it never restarts for a
// release the runner will not install, and never ignores one the runner will
// install.
func TestSignedReleaseRefused(t *testing.T) {
	t.Parallel()

	body := descriptor(t, newerRelease)
	valid := sig(testSigner, body)
	withField := func(k string, v any) []byte {
		var d map[string]any
		if err := json.Unmarshal(body, &d); err != nil {
			t.Fatal(err)
		}
		d[k] = v
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	oldSchema := withField("schemaVersion", 1)
	future := withField("version", uint64(time.Now().Add(48*time.Hour).Unix()))
	tampered := withField("version", 3000)

	many := make([]testSig, 0, 33)
	for range 33 {
		many = append(many, testSig{})
	}
	// A valid set padded with entries of other keys to 1 KiB.
	padded := sigSet(t, valid, testSig{KeyID: otherPubHex, Sig: strings.Repeat("0", 1024)})
	padded = padded[:updatecheck.MaxSignatureSize]

	for _, tc := range []struct {
		name      string
		body, sig []byte
		want      error
	}{
		{"no signature from the key", body, sigSet(t, sig(otherSigner, body)), updatecheck.ErrNotSigned},
		{"empty set", body, []byte{}, updatecheck.ErrBadSignatures},
		{"key listed twice", body, sigSet(t, valid, valid), updatecheck.ErrBadSignatures},
		{"over 32 signatures", body, sigSet(t, many...), updatecheck.ErrBadSignatures},
		{"signature not hex", body, sigSet(t, testSig{KeyID: testPubHex, Sig: "zz"}), updatecheck.ErrBadSignatures},
		{"tampered descriptor", tampered, sigSet(t, valid), updatecheck.ErrBadSignatures},
		{"schema version", oldSchema, sigSet(t, sig(testSigner, oldSchema)), updatecheck.ErrSchemaVersion},
		{"version far ahead", future, sigSet(t, sig(testSigner, future)), updatecheck.ErrVersionAhead},
		{"signature set of 1 KiB", body, padded, updatecheck.ErrResponseTooLarge},
		{"descriptor of 4 MiB", bytes.Repeat([]byte{' '}, updatecheck.MaxDescriptorSize), sigSet(t, valid), updatecheck.ErrResponseTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := newRegistry()
			reg.set("/release.json", response{body: tc.body})
			reg.set("/release.sig", response{body: tc.sig})
			s, err := updatecheck.NewUnstarted(log.Noop, runnerOptions(reg))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Check(context.Background()); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// A release key that is not in canonical form is not used. The check falls back
// to the unsigned /info, and the update restart stays inactive.
func TestInvalidReleaseKey(t *testing.T) {
	t.Parallel()

	for _, key := range []string{strings.ToUpper(testPubHex), " " + testPubHex, testPubHex[:62], "zz"} {
		reg := newRegistry()
		reg.offer(t, newerRelease)
		o := runnerOptions(reg)
		o.Runner.Pubkey = key
		o.Restart = updatecheck.RestartOptions{Enabled: true, DataDir: t.TempDir(), Shutdown: func() {}}

		s, err := updatecheck.NewUnstarted(log.Noop, o)
		if err != nil {
			t.Fatal(err)
		}
		if s.RestartActive() {
			t.Fatalf("key %q: restart active", key)
		}
		if _, err := s.Check(context.Background()); err != nil {
			t.Fatal(err)
		}
		if reg.count("/release.json") != 0 || reg.count("/info") != 1 {
			t.Fatalf("key %q: fetched the signed descriptor", key)
		}
	}
}

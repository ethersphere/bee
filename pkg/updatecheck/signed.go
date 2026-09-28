// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// descriptorSchemaVersion and maxVersionAhead mirror bee-runner's checks.
	descriptorSchemaVersion = 2
	maxVersionAhead         = 24 * time.Hour

	descriptorPath = "release.json"
	signaturePath  = "release.sig"

	// maxDescriptorSize and maxSignatureSize are bee-runner's limits: it
	// refuses a descriptor of 4 MiB or more and a signature set of 1 KiB or
	// more. Any other limit here would let a registry pad a valid release
	// so that bee restarts for a release the runner then refuses, or the
	// other way around.
	maxDescriptorSize = 4 << 20
	maxSignatureSize  = 1 << 10
	// maxSignatures bounds the entries of a signature set, as bee-runner
	// does.
	maxSignatures = 32
)

var (
	errNoReleaseKey  = errors.New("updatecheck: no usable release signing key")
	errBadSignatures = errors.New("updatecheck: invalid release signature set")
	errNotSigned     = errors.New("updatecheck: release descriptor is not signed by the release key")
	errSchemaVersion = errors.New("updatecheck: unsupported release descriptor schema version")
	errVersionAhead  = errors.New("updatecheck: release version is too far ahead of this node's clock")
)

// releaseKey is the release signing key from bee-runner. id is its lowercase
// hex encoding, which is also how a signature set names it.
type releaseKey struct {
	id  string
	pub ed25519.PublicKey
}

// parseReleaseKey parses the key bee-runner hands over. Only the canonical
// form is accepted: exactly 64 lowercase hex characters, no surrounding
// whitespace.
func parseReleaseKey(s string) (*releaseKey, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize || hex.EncodeToString(b) != s {
		return nil, fmt.Errorf("%w: not %d lowercase hex characters", errNoReleaseKey, 2*ed25519.PublicKeySize)
	}
	return &releaseKey{id: s, pub: ed25519.PublicKey(b)}, nil
}

// signature and sigSet mirror the registry's detached signature document:
// KeyID is the hex ed25519 public key, Sig the hex signature over the exact
// release.json bytes.
type signature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

type sigSet struct {
	Signatures []signature `json:"signatures"`
}

// verifySigSet checks that b holds a valid signature over body, the exact
// bytes served, by the release key. It mirrors bee-runner's verifier
// (verifySigSet in swarm-oci-registry internal/oci/signature.go) rule for
// rule, so that bee never restarts for a release the runner refuses, nor
// ignores one it accepts:
//   - the set must be non-empty, under 1 KiB (checked when it is fetched)
//     and at most maxSignatures entries, counting every entry;
//   - entries whose keyid is not the lowercase hex of the release key,
//     empty and junk ones included, are ignored;
//   - a second entry for the release key refuses the set;
//   - the release key's sig, trimmed of ASCII space, tab, CR and LF, must be
//     hex (either case) and verify, or the set is refused.
func (k *releaseKey) verifySigSet(body, b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("%w: empty", errBadSignatures)
	}
	if len(b) >= maxSignatureSize {
		return fmt.Errorf("%w: %d bytes, at most %d allowed", errBadSignatures, len(b), maxSignatureSize-1)
	}
	var s sigSet
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("%w: %w", errBadSignatures, err)
	}
	if len(s.Signatures) > maxSignatures {
		return fmt.Errorf("%w: %d signatures, over the %d limit", errBadSignatures, len(s.Signatures), maxSignatures)
	}
	var found *signature
	for i := range s.Signatures {
		if s.Signatures[i].KeyID != k.id {
			continue
		}
		if found != nil {
			return fmt.Errorf("%w: key %s listed twice", errBadSignatures, shortKey(k.id))
		}
		found = &s.Signatures[i]
	}
	if found == nil {
		return fmt.Errorf("%w: no signature from key %s", errNotSigned, shortKey(k.id))
	}
	raw, err := hex.DecodeString(strings.Trim(found.Sig, " \t\r\n"))
	if err != nil {
		return fmt.Errorf("%w: signature from %s is not hex", errBadSignatures, shortKey(k.id))
	}
	if !ed25519.Verify(k.pub, body, raw) {
		return fmt.Errorf("%w: signature from release key %s does not verify", errBadSignatures, shortKey(k.id))
	}
	return nil
}

func shortKey(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// descriptor is the subset of the signed release descriptor used here.
// Unknown fields, such as the expiresAt of older descriptors, are ignored.
type descriptor struct {
	SchemaVersion int               `json:"schemaVersion"`
	Version       uint64            `json:"version"`
	Channels      []string          `json:"channels"`
	Notes         string            `json:"notes"`
	Tags          map[string]string `json:"tags"`
}

// verifyDescriptor verifies the release key's signature over body and, only then,
// parses body into the fields that decide an update.
func (k *releaseKey) verifyDescriptor(body, sig []byte, now time.Time) (*release, error) {
	if err := k.verifySigSet(body, sig); err != nil {
		return nil, err
	}
	var d descriptor
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("decode %s: %w", descriptorPath, err)
	}
	// Refuse what bee-runner refuses, so bee never restarts for a release the
	// runner will not install. A version far in the future matters most: the
	// restart marker would suppress every real release below it.
	if d.SchemaVersion != descriptorSchemaVersion {
		return nil, fmt.Errorf("%w: %d", errSchemaVersion, d.SchemaVersion)
	}
	if limit := now.Add(maxVersionAhead).Unix(); limit > 0 && d.Version > uint64(limit) {
		return nil, fmt.Errorf("%w: v%d", errVersionAhead, d.Version)
	}
	r := &release{
		Verified: true,
		Version:  d.Version,
		Channels: d.Channels,
		Notes:    d.Notes,
	}
	tags := make([]string, 0, len(d.Tags))
	for t := range d.Tags {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	for _, t := range tags {
		r.Tags = append(r.Tags, tagInfo{Tag: t})
	}
	return r, nil
}

// fetchSigned fetches release.json and release.sig and returns the release
// they describe once the release key's signature verifies.
func (s *Service) fetchSigned(ctx context.Context) (*release, error) {
	body, err := s.http.get(ctx, s.descriptorURL, maxDescriptorSize)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", descriptorPath, err)
	}
	sig, err := s.http.get(ctx, s.signatureURL, maxSignatureSize)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", signaturePath, err)
	}
	return s.trust.verifyDescriptor(body, sig, time.Now())
}

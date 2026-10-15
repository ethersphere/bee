// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ErrInvalidMessage is wrapped by every error that reports a peer message
// breaking the wire rules (proposal §6.2, §6.4): a field that does not decode
// strictly, a range outside the session scope, an unsolicited part, a broken
// paging cursor, too many parts. The stream handlers reset the stream and
// disconnect the peer on it. Errors that are not the peer's fault (a failing
// local store) do not wrap it.
var ErrInvalidMessage = errors.New("rangesync: invalid message")

// Status is the decoded form of a pb.Status: the summary of a node's
// neighborhood that peers gossip on the status stream. The audit fields of
// pb.Status are not decoded in Phase 1.
type Status struct {
	// Root is the node's root range: its overlay prefix of StorageRadius bits.
	Root fingerprint.Range
	// Summary is the summary of the node's elements in Root.
	Summary fingerprint.Summary
	// LeafBits is the depth of the node's buckets below Root; Root.Bits +
	// LeafBits is at most fingerprint.MaxBits.
	LeafBits uint8
	// Mapping is the ID of the node's Mapper (fingerprint.DefaultMapperID in
	// Phase 1). Peers with different values do not run sessions.
	Mapping uint32
}

// Items is the decoded form of a pb.Items part: one page of the IDs of the
// sender's elements in Range (proposal §6.3).
//
// The pages of one range form a stream in ascending ID order. A page lists
// IDs strictly after After, in strictly ascending order, all inside Range, at
// most ItemsPageSize of them. After is nil on the first page and the last ID
// listed so far on every later page. More reports that further pages of the
// stream follow in later messages; a page with More set lists at least one
// ID.
//
// A page covers the IDs in (After, last listed ID] when More is set, and all
// of Range after After when it is not (the last page). With Reply set, the
// receiver answers each page with its own IDs in the interval the page covers
// that the page does not list.
type Items struct {
	Range fingerprint.Range
	IDs   []fingerprint.ID
	After *fingerprint.ID
	More  bool
	Reply bool
}

// digestSize is the length of a summary digest on the wire.
const digestSize = len(fingerprint.Summary{}.Digest)

// invalid returns an error wrapping ErrInvalidMessage.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidMessage, fmt.Sprintf(format, args...))
}

// ParseRange decodes a range received from a peer. The decoding is strict: p
// must not be nil, p.Bits must be at most fingerprint.MaxBits, and p.Prefix
// must hold exactly ceil(p.Bits/8) bytes with every bit past p.Bits zero. The
// result is therefore canonical (as returned by fingerprint.NewRange). Errors
// wrap ErrInvalidMessage.
func ParseRange(p *pb.Range) (fingerprint.Range, error) {
	if p == nil {
		return fingerprint.Range{}, invalid("missing range")
	}
	if p.Bits > fingerprint.MaxBits {
		return fingerprint.Range{}, invalid("range of %d bits", p.Bits)
	}
	bits := int(p.Bits)
	if len(p.Prefix) != (bits+7)/8 {
		return fingerprint.Range{}, invalid("range of %d bits with a %d byte prefix", bits, len(p.Prefix))
	}
	if rem := bits % 8; rem != 0 && p.Prefix[len(p.Prefix)-1]&(0xff>>rem) != 0 {
		return fingerprint.Range{}, invalid("range of %d bits with bits set past them", bits)
	}
	return fingerprint.NewRange(swarm.NewAddress(p.Prefix), uint8(bits)), nil
}

// ParseID decodes an element ID received from a peer: p must not be nil and
// p.Address and p.StampHash must hold exactly 32 bytes each
// (fingerprint.ParseID). Errors wrap ErrInvalidMessage.
func ParseID(p *pb.ID) (fingerprint.ID, error) {
	if p == nil {
		return fingerprint.ID{}, invalid("missing id")
	}
	id, err := fingerprint.ParseID(p.Address, p.StampHash)
	if err != nil {
		return fingerprint.ID{}, fmt.Errorf("%w: %w", ErrInvalidMessage, err)
	}
	return id, nil
}

// parseSummary decodes a count and its digest: the digest is empty when the
// count is 0 and exactly 32 bytes otherwise.
func parseSummary(count uint64, digest []byte) (fingerprint.Summary, error) {
	s := fingerprint.Summary{Count: count}
	if count == 0 {
		if len(digest) != 0 {
			return s, invalid("digest of %d bytes with count 0", len(digest))
		}
		return s, nil
	}
	if len(digest) != digestSize {
		return s, invalid("digest of %d bytes", len(digest))
	}
	copy(s.Digest[:], digest)
	return s, nil
}

// encodeDigest returns the wire form of the digest of s: omitted when the
// count is 0.
func encodeDigest(s fingerprint.Summary) []byte {
	if s.Count == 0 {
		return nil
	}
	return bytes.Clone(s.Digest[:])
}

// ParseFingerprint decodes a Fingerprint part: p and its range must not be
// nil, the range is decoded by ParseRange, and p.Digest must be empty when
// p.Count is 0 and exactly 32 bytes otherwise. A zero count decodes to the
// zero Summary. Errors wrap ErrInvalidMessage.
func ParseFingerprint(p *pb.Fingerprint) (fingerprint.Range, fingerprint.Summary, error) {
	if p == nil {
		return fingerprint.Range{}, fingerprint.Summary{}, invalid("missing fingerprint")
	}
	r, err := ParseRange(p.Range)
	if err != nil {
		return fingerprint.Range{}, fingerprint.Summary{}, fmt.Errorf("fingerprint: %w", err)
	}
	s, err := parseSummary(p.Count, p.Digest)
	if err != nil {
		return fingerprint.Range{}, fingerprint.Summary{}, fmt.Errorf("fingerprint %s: %w", r, err)
	}
	return r, s, nil
}

// ParseItems decodes an Items part and checks every rule of a page that
// needs no session state (see Items): p and its range must not be nil and the
// range decodes by ParseRange; p.After is empty or exactly 64 bytes
// (fingerprint.IDSize) whose address lies in the range; every ID decodes by
// ParseID, lies in the range, sorts strictly after After and after the
// previous ID; at most ItemsPageSize IDs, and at least one when p.More is
// set. Errors wrap ErrInvalidMessage.
func ParseItems(p *pb.Items) (Items, error) {
	if p == nil {
		return Items{}, invalid("missing items")
	}
	r, err := ParseRange(p.Range)
	if err != nil {
		return Items{}, fmt.Errorf("items: %w", err)
	}
	if len(p.IDs) > ItemsPageSize {
		return Items{}, invalid("items %s: page of %d ids", r, len(p.IDs))
	}
	if p.More && len(p.IDs) == 0 {
		return Items{}, invalid("items %s: empty page with more", r)
	}
	it := Items{Range: r, More: p.More, Reply: p.Reply}
	switch len(p.After) {
	case 0:
	case fingerprint.IDSize:
		var after fingerprint.ID
		copy(after[:], p.After)
		if !r.ContainsID(after) {
			return Items{}, invalid("items %s: cursor outside the range", r)
		}
		it.After = &after
	default:
		return Items{}, invalid("items %s: cursor of %d bytes", r, len(p.After))
	}
	if len(p.IDs) > 0 {
		it.IDs = make([]fingerprint.ID, len(p.IDs))
	}
	prev := it.After
	for i, pid := range p.IDs {
		id, err := ParseID(pid)
		if err != nil {
			return Items{}, fmt.Errorf("items %s: %w", r, err)
		}
		if !r.ContainsID(id) {
			return Items{}, invalid("items %s: id outside the range", r)
		}
		if prev != nil && id.Compare(*prev) <= 0 {
			return Items{}, invalid("items %s: ids out of order", r)
		}
		it.IDs[i] = id
		prev = &it.IDs[i]
	}
	return it, nil
}

// ParseStatus decodes a Status received from a peer: p.Root must not be nil
// and decodes by ParseRange; p.Digest must be empty when p.Count is 0 and
// exactly 32 bytes otherwise; Root.Bits + LeafBits must be at most
// fingerprint.MaxBits. LeafBits is a uint32 on the wire, so the sum is
// computed in uint64 (an int sum wraps on 32-bit platforms such as 386 and
// arm, which bee is released for). Mapping is returned as is: whether it
// matches is the caller's decision. AuditRound and AuditDigest are ignored in
// Phase 1. Errors wrap ErrInvalidMessage.
func ParseStatus(p *pb.Status) (Status, error) {
	if p == nil {
		return Status{}, invalid("missing status")
	}
	root, err := ParseRange(p.Root)
	if err != nil {
		return Status{}, fmt.Errorf("status root: %w", err)
	}
	if uint64(root.Bits)+uint64(p.LeafBits) > fingerprint.MaxBits {
		return Status{}, invalid("status: root of %d bits with %d leaf bits", root.Bits, p.LeafBits)
	}
	s, err := parseSummary(p.Count, p.Digest)
	if err != nil {
		return Status{}, fmt.Errorf("status: %w", err)
	}
	return Status{Root: root, Summary: s, LeafBits: uint8(p.LeafBits), Mapping: p.Mapping}, nil
}

// EncodeRange returns the wire form of r. The prefix is canonical, so
// ParseRange accepts the result and returns a range Equal to r.
func EncodeRange(r fingerprint.Range) *pb.Range {
	c := fingerprint.NewRange(r.Prefix, r.Bits)
	return &pb.Range{Prefix: c.Prefix.Bytes(), Bits: uint32(c.Bits)}
}

// EncodeID returns the wire form of id.
func EncodeID(id fingerprint.ID) *pb.ID {
	return &pb.ID{Address: bytes.Clone(id[:swarm.HashSize]), StampHash: bytes.Clone(id[swarm.HashSize:])}
}

// EncodeFingerprint returns the Fingerprint part of range r with summary s.
// The digest is omitted when s.Count is 0.
func EncodeFingerprint(r fingerprint.Range, s fingerprint.Summary) *pb.Fingerprint {
	return &pb.Fingerprint{Range: EncodeRange(r), Count: s.Count, Digest: encodeDigest(s)}
}

// EncodeItems returns the wire form of the page it. After is omitted when it
// is nil.
func EncodeItems(it Items) *pb.Items {
	p := &pb.Items{Range: EncodeRange(it.Range), More: it.More, Reply: it.Reply}
	if len(it.IDs) > 0 {
		p.IDs = make([]*pb.ID, len(it.IDs))
		for i, id := range it.IDs {
			p.IDs[i] = EncodeID(id)
		}
	}
	if it.After != nil {
		p.After = bytes.Clone(it.After[:])
	}
	return p
}

// EncodeStatus returns the wire form of st, with the digest omitted when
// st.Summary.Count is 0 and the audit fields empty.
func EncodeStatus(st Status) *pb.Status {
	return &pb.Status{
		Root:     EncodeRange(st.Root),
		Count:    st.Summary.Count,
		Digest:   encodeDigest(st.Summary),
		LeafBits: uint32(st.LeafBits),
		Mapping:  st.Mapping,
	}
}

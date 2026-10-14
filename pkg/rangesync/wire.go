// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
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

// ParseRange decodes a range received from a peer. The decoding is strict: p
// must not be nil, p.Bits must be at most fingerprint.MaxBits, and p.Prefix
// must hold exactly ceil(p.Bits/8) bytes with every bit past p.Bits zero. The
// result is therefore canonical (as returned by fingerprint.NewRange). Errors
// wrap ErrInvalidMessage.
func ParseRange(p *pb.Range) (fingerprint.Range, error) {
	return fingerprint.Range{}, errNotImplemented
}

// ParseID decodes an element ID received from a peer: p must not be nil and
// p.Address and p.StampHash must hold exactly 32 bytes each
// (fingerprint.ParseID). Errors wrap ErrInvalidMessage.
func ParseID(p *pb.ID) (fingerprint.ID, error) {
	return fingerprint.ID{}, errNotImplemented
}

// ParseFingerprint decodes a Fingerprint part: p and its range must not be
// nil, the range is decoded by ParseRange, and p.Digest must be empty when
// p.Count is 0 and exactly 32 bytes otherwise. A zero count decodes to the
// zero Summary. Errors wrap ErrInvalidMessage.
func ParseFingerprint(p *pb.Fingerprint) (fingerprint.Range, fingerprint.Summary, error) {
	return fingerprint.Range{}, fingerprint.Summary{}, errNotImplemented
}

// ParseItems decodes an Items part and checks every rule of a page that
// needs no session state (see Items): p and its range must not be nil and the
// range decodes by ParseRange; p.After is empty or exactly 64 bytes
// (fingerprint.IDSize) whose address lies in the range; every ID decodes by
// ParseID, lies in the range, sorts strictly after After and after the
// previous ID; at most ItemsPageSize IDs, and at least one when p.More is
// set. Errors wrap ErrInvalidMessage.
func ParseItems(p *pb.Items) (Items, error) {
	return Items{}, errNotImplemented
}

// ParseStatus decodes a Status received from a peer: p.Root must not be nil
// and decodes by ParseRange; p.Digest must be empty when p.Count is 0 and
// exactly 32 bytes otherwise; Root.Bits + LeafBits must be at most
// fingerprint.MaxBits. LeafBits is a uint32 on the wire, so the sum is
// computed in uint64 (an int sum wraps on 32-bit platforms such as 386 and
// arm, which bee is released for). Mapping is returned as is:
// whether it matches is the caller's decision. AuditRound and AuditDigest are
// ignored in Phase 1. Errors wrap ErrInvalidMessage.
func ParseStatus(p *pb.Status) (Status, error) {
	return Status{}, errNotImplemented
}

// EncodeRange returns the wire form of r. The prefix is canonical, so
// ParseRange accepts the result and returns a range Equal to r.
func EncodeRange(r fingerprint.Range) *pb.Range {
	return nil
}

// EncodeID returns the wire form of id.
func EncodeID(id fingerprint.ID) *pb.ID {
	return nil
}

// EncodeFingerprint returns the Fingerprint part of range r with summary s.
// The digest is omitted when s.Count is 0.
func EncodeFingerprint(r fingerprint.Range, s fingerprint.Summary) *pb.Fingerprint {
	return nil
}

// EncodeItems returns the wire form of the page it. After is omitted when it
// is nil.
func EncodeItems(it Items) *pb.Items {
	return nil
}

// EncodeStatus returns the wire form of st, with the digest omitted when
// st.Summary.Count is 0 and the audit fields empty.
func EncodeStatus(st Status) *pb.Status {
	return nil
}

// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"crypto/sha256"
	"errors"
	"math/rand/v2"
	"slices"
	"sort"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// fakeLocal is an in-memory rangesync.Local over a sorted set of IDs. Its
// digest is the XOR of sha256(id) over the range, which is enough to tell
// sets apart in tests.
type fakeLocal struct {
	ids []fingerprint.ID // sorted, unique

	// summary, if set, overrides Summary.
	summary func(r fingerprint.Range) (fingerprint.Summary, error)
	// exhausted is returned by Symbols.
	exhausted bool
	// status, if set, makes Symbols serve the windows the Index serves for
	// that Status: a stream of a range holding more than ScanLimit elements
	// ends at its cache (rangesync.CacheCapacity).
	status *rangesync.Status
	// symbolLimit, if not zero, ends every stream at that many symbols.
	symbolLimit uint64
	// extra are encoded into every symbol window, whatever its range, but
	// are not elements of l (a corrupt stream).
	extra []fingerprint.ID
	// symbolCalls counts the Symbols calls.
	symbolCalls int
	// err, if set, is returned by every method.
	err error

	hasCalls int
}

var _ rangesync.Local = (*fakeLocal)(nil)

func newFakeLocal(ids ...fingerprint.ID) *fakeLocal {
	ids = slices.Clone(ids)
	slices.SortFunc(ids, fingerprint.ID.Compare)
	return &fakeLocal{ids: slices.Compact(ids)}
}

// start returns the index of the first ID in r.
func (l *fakeLocal) start(r fingerprint.Range) int {
	var lo fingerprint.ID
	c := fingerprint.NewRange(r.Prefix, r.Bits)
	copy(lo[:], c.Prefix.Bytes())
	return sort.Search(len(l.ids), func(i int) bool { return l.ids[i].Compare(lo) >= 0 })
}

func (l *fakeLocal) Summary(r fingerprint.Range) (fingerprint.Summary, error) {
	if l.err != nil {
		return fingerprint.Summary{}, l.err
	}
	if l.summary != nil {
		return l.summary(r)
	}
	var s fingerprint.Summary
	for i := l.start(r); i < len(l.ids) && r.ContainsID(l.ids[i]); i++ {
		s.Count++
		h := sha256.Sum256(l.ids[i][:])
		for j := range s.Digest {
			s.Digest[j] ^= h[j]
		}
	}
	if s.Count == 0 {
		s.Digest = [32]byte{}
	}
	return s, nil
}

func (l *fakeLocal) IterateIDs(r fingerprint.Range, after *fingerprint.ID, fn func(fingerprint.ID) (bool, error)) error {
	if l.err != nil {
		return l.err
	}
	i := l.start(r)
	if after != nil {
		j := sort.Search(len(l.ids), func(i int) bool { return l.ids[i].Compare(*after) > 0 })
		i = max(i, j)
	}
	for ; i < len(l.ids) && r.ContainsID(l.ids[i]); i++ {
		stop, err := fn(l.ids[i])
		if err != nil || stop {
			return err
		}
	}
	return nil
}

func (l *fakeLocal) HasID(id fingerprint.ID) (bool, error) {
	if l.err != nil {
		return false, l.err
	}
	l.hasCalls++
	_, ok := slices.BinarySearchFunc(l.ids, id, fingerprint.ID.Compare)
	return ok, nil
}

func (l *fakeLocal) Symbols(r fingerprint.Range, start, end uint64) ([]fingerprint.CodedSymbol, bool, error) {
	if l.err != nil {
		return nil, false, l.err
	}
	if end < start {
		return nil, false, fingerprint.ErrInvalidBatch
	}
	l.symbolCalls++
	in := l.in(r)
	exhausted := l.exhausted
	limit := uint64(rangesync.CapUnlimited)
	if l.status != nil && len(in) > fingerprint.ScanLimit {
		limit = rangesync.CacheCapacity(*l.status, r)
	}
	if l.symbolLimit != 0 {
		limit = min(limit, l.symbolLimit)
	}
	if end >= limit {
		end, exhausted = limit, true
	}
	start = min(start, end)
	syms := make([]fingerprint.CodedSymbol, end-start)
	for _, id := range append(in, l.extra...) {
		fingerprint.Encode(fingerprint.NewFloatMapper(), syms, start, id, 1)
	}
	return syms, exhausted, nil
}

// in returns the IDs of l in r.
func (l *fakeLocal) in(r fingerprint.Range) []fingerprint.ID {
	var out []fingerprint.ID
	for _, id := range l.ids {
		if r.ContainsID(id) {
			out = append(out, id)
		}
	}
	return out
}

func newRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

// randID returns a random ID whose address lies in r.
func randID(rng *rand.Rand, r fingerprint.Range) fingerprint.ID {
	var id fingerprint.ID
	for i := range id {
		id[i] = byte(rng.Uint32())
	}
	p := r.Prefix.Bytes()
	for k := 0; k < int(r.Bits); k++ {
		mask := byte(0x80) >> (k % 8)
		id[k/8] = id[k/8]&^mask | p[k/8]&mask
	}
	return id
}

func randIDs(rng *rand.Rand, r fingerprint.Range, n int) []fingerprint.ID {
	ids := make([]fingerprint.ID, n)
	for i := range ids {
		ids[i] = randID(rng, r)
	}
	return ids
}

// rangeOf returns the canonical range of the first bits bits of the binary
// string s, for example rangeOf("0110") is 0110/4.
func rangeOf(s string) fingerprint.Range {
	p := make([]byte, swarm.HashSize)
	for k, c := range s {
		if c == '1' {
			p[k/8] |= 0x80 >> (k % 8)
		}
	}
	return fingerprint.NewRange(swarm.NewAddress(p), uint8(len(s)))
}

func idPtr(id fingerprint.ID) *fingerprint.ID { return &id }

func fpPart(r fingerprint.Range, s fingerprint.Summary) *pb.Part {
	return &pb.Part{Kind: &pb.Part_FP{FP: rangesync.EncodeFingerprint(r, s)}}
}

func itemsPart(it rangesync.Items) *pb.Part {
	return &pb.Part{Kind: &pb.Part_Items{Items: rangesync.EncodeItems(it)}}
}

func msg(parts ...*pb.Part) *pb.Message { return &pb.Message{Parts: parts} }

// wire checks that m is within the message limits and returns it after a
// round trip through its encoding, as the peer receives it.
func wire(t *testing.T, m *pb.Message) *pb.Message {
	t.Helper()
	if len(m.Parts) > rangesync.MaxParts {
		t.Fatalf("message has %d parts, over %d", len(m.Parts), rangesync.MaxParts)
	}
	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > rangesync.MaxMessageBytes {
		t.Fatalf("message has %d bytes, over %d", len(data), rangesync.MaxMessageBytes)
	}
	got := new(pb.Message)
	if err := got.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	return got
}

// drive runs a session between initiator a, whose opening Message is open,
// and responder b until one side ends it, and returns the wants of each side
// and the number of messages sent. Every message goes through wire.
//
// inspect, if given, is called with every message and whether the initiator
// sent it.
func drive(t *testing.T, a *rangesync.Session, open *pb.Message, b *rangesync.Session, inspect ...func(fromInitiator bool, m *pb.Message)) (wantsA, wantsB []fingerprint.ID, messages int) {
	t.Helper()
	sides := [2]*rangesync.Session{b, a}
	wants := [2][]fingerprint.ID{}
	m := open
	for i := 0; ; i++ {
		if i > 100000 {
			t.Fatal("session does not end")
		}
		messages++
		for _, f := range inspect {
			f(i%2 == 0, m)
		}
		out, w, err := sides[i%2].Handle(wire(t, m))
		if err != nil {
			t.Fatalf("message %d: %v", messages, err)
		}
		wants[i%2] = append(wants[i%2], w...)
		if out == nil {
			return wants[1], wants[0], messages
		}
		m = out
	}
}

// partsOf returns the Fingerprint ranges and the Items pages of m; Symbols
// and More parts are skipped.
func partsOf(t *testing.T, m *pb.Message) (fps []fingerprint.Range, items []rangesync.Items) {
	t.Helper()
	for _, p := range m.GetParts() {
		switch {
		case p.GetFP() != nil:
			r, _, err := rangesync.ParseFingerprint(p.GetFP())
			if err != nil {
				t.Fatal(err)
			}
			fps = append(fps, r)
		case p.GetItems() != nil:
			it, err := rangesync.ParseItems(p.GetItems())
			if err != nil {
				t.Fatal(err)
			}
			items = append(items, it)
		case p.GetSymbols() != nil, p.GetMore() != nil:
		default:
			t.Fatalf("unexpected part %v", p)
		}
	}
	return fps, items
}

func status(r fingerprint.Range, leafBits uint8) rangesync.Status {
	return rangesync.Status{Root: r, LeafBits: leafBits, Mapping: fingerprint.DefaultMapperID}
}

func mustScope(t *testing.T, local, remote rangesync.Status) rangesync.Scope {
	t.Helper()
	s, err := rangesync.NewScope(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func isInvalid(err error) bool { return errors.Is(err, rangesync.ErrInvalidMessage) }

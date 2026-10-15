// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"

	"golang.org/x/crypto/sha3"
)

// Rateless IBLT (Yang, Gilad, Alizadeh, SIGCOMM 2024), proposal §4.3.4–§4.3.7.
//
// A set of IDs is encoded as an unbounded stream of coded symbols. A Mapper
// decides which symbols each ID is mapped to; nothing else in this package
// computes symbol indices or the mapping probability ρ. Encode, the Decoder
// and the wire codec (AppendSymbolData, ParseSymbolData) all take a Mapper,
// and so do the symbol caches of the Index.

const (
	// ChecksumSize is the size of the checksum of an ID in bytes (proposal
	// §9: 128 bits, so a crafted collision needs ~2^64 work).
	ChecksumSize = 16

	// MinSymbolSize and MaxSymbolSize bound the encoded size of one coded
	// symbol in Symbols.Data: the sum, the checksum and a varint of 1 to
	// binary.MaxVarintLen64 bytes.
	MinSymbolSize = IDSize + ChecksumSize + 1
	MaxSymbolSize = IDSize + ChecksumSize + binary.MaxVarintLen64

	// EndIndex is returned by IndexIterator.Next when a mapping has no further
	// indices. It is never a symbol index.
	EndIndex uint64 = math.MaxUint64
)

// Errors returned by the rateless coder.
var (
	// ErrCorrupt is returned by Decoder.Add when the symbol stream cannot be
	// the difference of two sets in the decoder's range (see Decoder). The
	// session falls back to an exact item listing of the range (proposal §9).
	ErrCorrupt = errors.New("fingerprint: corrupt symbol stream")
	// ErrInvalidBatch is returned by Decoder.Add for a batch that does not
	// continue the stream: start is not Received(), remote and local differ
	// in length, or the decoder is already done. The decoder is unchanged.
	ErrInvalidBatch = errors.New("fingerprint: invalid symbol batch")
	// ErrInvalidSymbolData is returned by ParseSymbolData for data that is not
	// a canonical encoding of whole coded symbols.
	ErrInvalidSymbolData = errors.New("fingerprint: invalid symbol data")
)

// checksumBytes is the number of checksum bytes Checksum keeps; the rest are
// zero. It is ChecksumSize except in tests that truncate the checksum to
// provoke collisions.
//
// checksumBytes and peelFactor are package variables only so that tests can
// override them (SetChecksumBytes, SetPeelFactor in export_test.go); tests
// that change them must not run in parallel.
var checksumBytes = ChecksumSize

// peelFactor caps the number of recovered ids at peelFactor × Received()
// (proposal §6.4). Tests lower it.
var peelFactor uint64 = 2

var (
	checksumPrefix = []byte("rangesync/check")
	mapPrefix      = []byte("rangesync/map")
)

// keccak256 returns the legacy Keccak-256 of prefix ‖ id.
func keccak256(prefix []byte, id *ID) (out [32]byte) {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(prefix)
	_, _ = h.Write(id[:])
	h.Sum(out[:0])
	return out
}

// Checksum returns the 128-bit checksum of id: the first ChecksumSize bytes of
// keccak256("rangesync/check" ‖ id), with keccak256 the legacy Keccak used by
// swarm hashing. The checksum is fixed, not keyed, so cached symbols stay
// valid across sessions (proposal §9, "Why not keyed checksums?").
//
// Together with id it forms the coded symbol of the one-element set {id}:
// CodedSymbol{Sum: id, Check: Checksum(id), Count: 1}. Callers that add one
// id to many symbol arrays (the Index caches) compute it once and add it with
// CodedSymbol.Add at each index of Mapper.Indices(id).
func Checksum(id ID) [ChecksumSize]byte {
	var c [ChecksumSize]byte
	h := keccak256(checksumPrefix, &id)
	copy(c[:checksumBytes], h[:])
	return c
}

// CodedSymbol is one symbol of a Rateless IBLT stream (proposal §4.3.4): the
// XOR of the IDs mapped to it, the XOR of their checksums, and how many there
// are. After a subtraction Count is signed: the number of IDs on one side
// minus the number on the other.
//
// Coded symbols form a commutative group (XOR on Sum and Check, two's
// complement addition on Count, which wraps on overflow), so *CodedSymbol
// satisfies Group and symbols of disjoint sets combine symbol by symbol. The
// zero value is the symbol of the empty set.
type CodedSymbol struct {
	// Sum is the XOR of the IDs mapped to the symbol.
	Sum ID
	// Check is the XOR of their checksums.
	Check [ChecksumSize]byte
	// Count is the number of IDs mapped to the symbol.
	Count int64
}

// Add sets s to s + o: Sum and Check are XORed, Count is added.
func (s *CodedSymbol) Add(o *CodedSymbol) {
	s.xor(o)
	s.Count += o.Count
}

// Sub sets s to s − o: Sum and Check are XORed, Count is subtracted.
func (s *CodedSymbol) Sub(o *CodedSymbol) {
	s.xor(o)
	s.Count -= o.Count
}

// Zero sets s to the symbol of the empty set.
func (s *CodedSymbol) Zero() {
	*s = CodedSymbol{}
}

func (s *CodedSymbol) xor(o *CodedSymbol) {
	for k := 0; k < IDSize; k += 8 {
		binary.LittleEndian.PutUint64(s.Sum[k:], binary.LittleEndian.Uint64(s.Sum[k:])^binary.LittleEndian.Uint64(o.Sum[k:]))
	}
	for k := 0; k < ChecksumSize; k += 8 {
		binary.LittleEndian.PutUint64(s.Check[k:], binary.LittleEndian.Uint64(s.Check[k:])^binary.LittleEndian.Uint64(o.Check[k:]))
	}
}

func (s *CodedSymbol) isZero() bool {
	return *s == CodedSymbol{}
}

// CodedSymbol is the element type of the bucket symbol Fenwick tree (T2.2
// adapts arrays of it).
var _ = NewFenwick[CodedSymbol, *CodedSymbol]

// Mapper defines how IDs are spread over the symbols of a stream (proposal
// §4.3.4, §5.2). It is pluggable so that the float64 mapper can be replaced
// by an integer one without changing anything else. Every function of a
// Mapper is pure and must give identical results on every node that reports
// the same ID().
type Mapper interface {
	// ID identifies the mapping. It is sent in Status.Mapping, and peers
	// run sessions only when their IDs are equal.
	ID() uint32
	// Indices returns the symbol indices id is mapped to, in increasing
	// order, starting at 0: every id is in symbol 0 (ρ(0) = 1), which is what
	// makes "symbol 0 is empty" the stop rule of the Decoder.
	Indices(id ID) IndexIterator
	// Expected returns round(count·ρ(i)): about the expected Count of symbol
	// i in the stream of a set of count elements, where ρ(i) approximates the
	// probability that an id is mapped to symbol i (the gap sampling of a
	// mapper need not hit ρ exactly; FloatMapper gives 0.64 at i = 1 against
	// ρ(1) = 2/3). The wire codec sends a symbol's Count as its difference
	// from Expected, so Expected only affects the encoded size, never
	// correctness, but both ends must compute the same value.
	Expected(count, i uint64) int64
}

// IndexIterator yields the symbol indices of one id.
type IndexIterator interface {
	// Next returns the next index. Indices are strictly increasing and the
	// first is 0. A mapping may end: from then on Next returns EndIndex.
	// Callers stop at their own bound, which is always below EndIndex.
	Next() uint64
}

// FloatMapper is the first Mapper (ID DefaultMapperID), the float64 mapping
// of proposal §4.3.4 with ρ(i) = 1/(1 + i/2). The zero value is ready to use.
//
// Indices of id: seed = keccak256("rangesync/map" ‖ id) (legacy Keccak) seeds
// a xoshiro256** 1.0 generator (Blackman and Vigna: multipliers 5 and 9,
// rotation 7 for the output, shift 17 and rotation 45 for the state update,
// output taken from the state before it advances) whose 256-bit state is the
// seed read as four little-endian uint64 words, in order. The first index is
// 0. From index i, the next is i + gap with
//
//	x   = the next xoshiro256** output
//	w   = float64((x >> 11) + 1) · 2^-53       // w = 1 − u, u uniform in [0, 1)
//	t   = 1 / math.Sqrt(w)                     // (1 − u)^(−1/2)
//	gap = max(1, ⌈(1.5 + float64(i)) · (t − 1)⌉)
//
// The gap is at least 1 so that indices increase strictly (u = 0 would give
// 0). When i + gap would exceed 2^53 − 1, the largest integer a float64 holds
// exactly, the mapping ends and Next returns EndIndex.
//
// Expected(count, i) = math.Round(float64(count)·2 / (float64(i) + 2)),
// which is round(count·ρ(i)) with a single rounded division. Ties round away
// from zero (math.Round, not round-half-to-even): Expected(1, 2) = 1 and
// Expected(5, 2) = 3. A quotient of 2^63 or more (float64(math.MaxInt64) is
// 2^63) is clamped to math.MaxInt64.
//
// Determinism. Nodes with the same mapper ID must compute bit-identical
// indices and expected counts, but Go does not promise identical float64
// results across architectures: the spec lets a compiler fuse x*y + z into a
// fused multiply-add unless the product is explicitly converted. So every
// step above is a separate statement whose result is explicitly converted with
// float64(...); no product is added to anything; every operation is a single
// IEEE 754 operation rounded to nearest; square roots use math.Sqrt, which is
// correctly rounded on every architecture, never math.Pow; and every
// float-to-integer conversion is range-checked first, because out-of-range
// conversions are implementation-defined. The testdata vectors are checked on
// amd64 only; other architectures are T2.8 (D22). Any change to these
// formulas changes the mapping and needs a new mapper ID.
type FloatMapper struct{}

var _ Mapper = FloatMapper{}

// NewFloatMapper returns the default Mapper, a FloatMapper.
func NewFloatMapper() FloatMapper {
	return FloatMapper{}
}

// ID returns DefaultMapperID.
func (FloatMapper) ID() uint32 {
	return DefaultMapperID
}

// Indices returns the symbol indices of id (see FloatMapper).
func (FloatMapper) Indices(id ID) IndexIterator {
	seed := keccak256(mapPrefix, &id)
	it := &floatIterator{}
	for k := range it.s {
		it.s[k] = binary.LittleEndian.Uint64(seed[8*k:])
	}
	return it
}

// maxFloatIndex is the largest index of a FloatMapper mapping: 2^53 − 1, the
// largest integer a float64 holds exactly.
const maxFloatIndex = 1<<53 - 1

// floatIterator is the IndexIterator of FloatMapper.
type floatIterator struct {
	s       [4]uint64 // xoshiro256** state
	i       uint64    // the last index returned, or EndIndex
	started bool      // index 0 has been returned
}

// next returns the next xoshiro256** 1.0 output and advances the state.
func (it *floatIterator) next() uint64 {
	s := &it.s
	result := bits.RotateLeft64(s[1]*5, 7) * 9
	t := s[1] << 17
	s[2] ^= s[0]
	s[3] ^= s[1]
	s[1] ^= s[2]
	s[0] ^= s[3]
	s[2] ^= t
	s[3] = bits.RotateLeft64(s[3], 45)
	return result
}

// Next returns the next index (see FloatMapper).
func (it *floatIterator) Next() uint64 {
	if !it.started {
		it.started = true
		it.i = 0
		return 0
	}
	if it.i == EndIndex {
		return EndIndex
	}
	// One IEEE 754 operation per statement, each result explicitly converted,
	// so that no compiler may fuse them (see FloatMapper).
	x := it.next()
	w := float64(float64((x>>11)+1) * 0x1p-53) // 1 − u in (0, 1], exact
	t := float64(1 / math.Sqrt(w))
	a := float64(1.5 + float64(it.i))
	b := float64(t - 1)
	p := float64(a * b)
	c := math.Ceil(p)
	// Range check before converting: a gap of 2^53 or more ends the mapping
	// anyway.
	if c >= 0x1p53 {
		it.i = EndIndex
		return EndIndex
	}
	gap := uint64(1)
	if c > 1 {
		gap = uint64(c)
	}
	if gap > maxFloatIndex-it.i {
		it.i = EndIndex
		return EndIndex
	}
	it.i += gap
	return it.i
}

// Expected returns round(count·ρ(i)) (see FloatMapper).
func (FloatMapper) Expected(count, i uint64) int64 {
	// Separate, explicitly converted statements (see FloatMapper).
	c := float64(count)
	num := float64(c * 2)
	den := float64(float64(i) + 2)
	q := float64(num / den)
	if q >= 0x1p63 {
		return math.MaxInt64
	}
	return int64(math.Round(q))
}

// Encode adds id with the given sign to the symbols syms, which hold symbols
// [start, start+len(syms)) of a stream: for every index i of m.Indices(id) in
// that window, syms[i-start] gets Sum and Check XORed with id and Checksum(id)
// and Count increased by sign. sign is +1 to add id and −1 to remove it.
// Encode panics if sign is neither or if start+len(syms) overflows a uint64.
//
// Encoding is linear: the symbols of a set are the sum of the symbols of its
// elements, so encode(A) − encode(B) = encode(A \ B) − encode(B \ A).
func Encode(m Mapper, syms []CodedSymbol, start uint64, id ID, sign int64) {
	if sign != 1 && sign != -1 {
		panic(fmt.Sprintf("fingerprint: Encode sign %d", sign))
	}
	end := windowEnd(start, len(syms))
	if len(syms) == 0 {
		return
	}
	one := CodedSymbol{Sum: id, Check: Checksum(id), Count: sign}
	it := m.Indices(id)
	for i := it.Next(); i < end; i = it.Next() {
		if i >= start {
			syms[i-start].Add(&one)
		}
	}
}

// windowEnd returns start+n and panics if it overflows a uint64.
func windowEnd(start uint64, n int) uint64 {
	end, carry := bits.Add64(start, uint64(n), 0)
	if carry != 0 {
		panic("fingerprint: symbol window overflows the index space")
	}
	return end
}

// AppendSymbolData appends the wire encoding of the coded symbols syms, which
// are symbols [start, start+len(syms)) of the sender's stream for a range
// holding count elements, to dst and returns the extended slice. This is
// Symbols.Data of proposal §6.2; count is Symbols.Count and start is
// Symbols.Start.
//
// Symbol k (absolute index i = start + k) is encoded as
//
//	Sum (64 B) ‖ Check (16 B) ‖ varint(zigzag(Count − m.Expected(count, i)))
//
// with the signed varint of encoding/binary (AppendVarint, which is zigzag
// encoded) and the difference computed with wrapping int64 arithmetic, so
// every Count round-trips exactly, whatever count is. A symbol takes
// MinSymbolSize to MaxSymbolSize bytes. AppendSymbolData panics if
// start+len(syms) overflows a uint64.
func AppendSymbolData(dst []byte, m Mapper, count, start uint64, syms []CodedSymbol) []byte {
	windowEnd(start, len(syms))
	dst = slices.Grow(dst, len(syms)*MinSymbolSize)
	for k := range syms {
		s := &syms[k]
		dst = append(dst, s.Sum[:]...)
		dst = append(dst, s.Check[:]...)
		dst = binary.AppendVarint(dst, s.Count-m.Expected(count, start+uint64(k)))
	}
	return dst
}

// ParseSymbolData decodes Symbols.Data as written by AppendSymbolData with the
// same Mapper, count and start. count is the received Symbols.Count (the
// sender's element count), not the local one. data is untrusted peer input:
// it must consist of whole symbols, each varint must be the shortest encoding
// of its value, and start plus the number of symbols must not overflow a
// uint64; otherwise it returns an error wrapping ErrInvalidSymbolData. So
// data is accepted only if AppendSymbolData would produce it exactly. Empty
// data decodes to no symbols.
//
// It allocates one CodedSymbol per MinSymbolSize bytes of data at most; the
// caller bounds len(data) (MaxMessageBytes). A peer that lies about count only
// makes its own encoding longer, so count is not validated here.
func ParseSymbolData(m Mapper, count, start uint64, data []byte) ([]CodedSymbol, error) {
	const head = IDSize + ChecksumSize
	syms := make([]CodedSymbol, 0, len(data)/MinSymbolSize)
	for i := start; len(data) > 0; i++ {
		// start plus the number of symbols must fit in a uint64, so the
		// index of a symbol is at most MaxUint64 − 1.
		if i == math.MaxUint64 {
			return nil, fmt.Errorf("%w: symbol %d: index overflows", ErrInvalidSymbolData, len(syms))
		}
		if len(data) < MinSymbolSize {
			return nil, fmt.Errorf("%w: symbol %d: %d bytes left", ErrInvalidSymbolData, len(syms), len(data))
		}
		var s CodedSymbol
		copy(s.Sum[:], data)
		copy(s.Check[:], data[IDSize:])
		v, n := binary.Varint(data[head:])
		if n <= 0 {
			return nil, fmt.Errorf("%w: symbol %d: bad count varint", ErrInvalidSymbolData, len(syms))
		}
		// A varint is the shortest encoding of its value unless it ends in a
		// zero continuation byte.
		if n > 1 && data[head+n-1] == 0 {
			return nil, fmt.Errorf("%w: symbol %d: non-minimal count varint", ErrInvalidSymbolData, len(syms))
		}
		s.Count = v + m.Expected(count, i)
		syms = append(syms, s)
		data = data[head+n:]
	}
	return syms, nil
}

// Decoder peels the difference between a peer's symbol stream for a range and
// the local stream for the same range (proposal §4.3.7). It is fed the
// stream batch by batch, from symbol 0 on, each batch together with the local
// symbols at the same indices, and works incrementally: symbols received
// earlier are kept and never wasted.
//
// The decoder works on d_i = local_i − remote_i. A symbol is pure when its
// Count is +1 or −1 and its Check equals Checksum(Sum); its Sum is then a
// recovered id, only local for +1 and only remote for −1. A recovered id is
// removed from every received symbol it maps to (and from symbols received
// later), which may make more symbols pure. Every id is in symbol 0, so
// symbol 0 is empty (the zero CodedSymbol) exactly when the whole difference
// has been recovered: that is Done.
//
// Precondition: remote and local are each the stream of one fixed set across
// all batches of the decoder. If either set changes between batches (an
// element added or evicted while the stream is read), Add may return
// ErrCorrupt or the decoder may never be Done. Callers serve a stream from
// one snapshot on each side.
//
// Corruption (proposal §6.4, §9). Add returns an error wrapping ErrCorrupt,
// and the decoder stays corrupt (later Adds return ErrCorrupt, Done is false),
// when
//   - (a) a pure symbol at index j yields an id whose mapping does not
//     contain j,
//   - a recovered id lies outside the decoder's range,
//   - an id is recovered a second time,
//   - more than 2 × Received() ids have been recovered (peel steps), or
//   - (b) symbol 0 is empty while another received symbol is not. Rule (b)
//     is checked only once the peeling of a batch has run to completion.
//
// Under the precondition, none of these happens for honest streams unless
// checksums collide, which takes ~2^64 work with 128-bit checksums. Rule (a)
// is the one that catches a checksum collision: a false pure symbol holds an
// odd number of ids, and the XOR of an odd number of ids in the range is
// again in the range, so the range rule only detects a malicious peer. A
// collision that rule (a) misses can still end in a wrong Done: the ids it
// yields are undeliverable on fetch and the range's fingerprints still
// differ, so a later round reconciles it again (§9). A corrupt decoder never
// reports Done; its partial Result must not be used.
//
// The decoder does not bound the number of symbols it keeps (one CodedSymbol
// per symbol received); the session does (SessionSymbolBudget). A Decoder is
// not safe for concurrent use.
type Decoder struct {
	m Mapper
	r Range

	syms    []CodedSymbol // d_i = local_i − remote_i, for the received symbols
	nonzero int           // number of non-zero symbols in syms

	peeled    []peeled        // recovered ids, to remove from later symbols
	seen      map[ID]struct{} // recovered ids
	onlyLocal []ID            // ids recovered from +1 symbols
	onlyRem   []ID            // ids recovered from −1 symbols
	queue     []uint64        // indices of symbols that may be pure
	scratch   []uint64        // indices of the id being recovered
	err       error           // sticky corruption
}

// peeled is a recovered id together with the part of its mapping beyond the
// received symbols, from which it is removed as more symbols arrive.
type peeled struct {
	sym  CodedSymbol // the coded symbol of the id, with Count its sign in d
	it   IndexIterator
	next uint64 // the next index of the mapping not yet removed
}

// NewDecoder returns a decoder for the symbol stream of range r under mapper
// m. Both peers must use mappers with the same ID; the decoder cannot detect
// a mismatch, which only shows as a stream that never decodes.
func NewDecoder(m Mapper, r Range) *Decoder {
	return &Decoder{m: m, r: r, seen: make(map[ID]struct{})}
}

// Add feeds the next batch: remote holds the peer's symbols [start,
// start+len(remote)) and local this node's symbols at the same indices.
//
// A corrupt decoder returns ErrCorrupt. Otherwise start must equal Received()
// and len(local) must equal len(remote) or be 0 (no local symbols: all empty);
// if not, Add returns an error wrapping ErrInvalidBatch and changes nothing.
// A batch without symbols (len(remote) == 0) is then a no-op that returns
// nil, also once Done; a non-empty batch once Done is an ErrInvalidBatch.
// Otherwise Add peels as far as the received symbols allow and returns an
// error wrapping ErrCorrupt if the stream is corrupt (see Decoder). It does
// not retain remote or local.
func (d *Decoder) Add(start uint64, remote, local []CodedSymbol) error {
	if d.err != nil {
		return d.err
	}
	received := d.Received()
	if start != received {
		return fmt.Errorf("%w: start %d, received %d", ErrInvalidBatch, start, received)
	}
	if len(local) != 0 && len(local) != len(remote) {
		return fmt.Errorf("%w: %d remote and %d local symbols", ErrInvalidBatch, len(remote), len(local))
	}
	if len(remote) == 0 {
		return nil
	}
	if d.Done() {
		return fmt.Errorf("%w: decoder is done", ErrInvalidBatch)
	}
	if uint64(len(remote)) >= EndIndex-received {
		return fmt.Errorf("%w: batch overflows the index space", ErrInvalidBatch)
	}

	// d_i = local_i − remote_i for the new symbols.
	for k := range remote {
		var s CodedSymbol
		if len(local) != 0 {
			s = local[k]
		}
		s.Sub(&remote[k])
		d.syms = append(d.syms, s)
		if !s.isZero() {
			d.nonzero++
		}
	}
	end := d.Received()

	// Remove the ids recovered earlier from the new symbols they map to.
	for k := range d.peeled {
		p := &d.peeled[k]
		for ; p.next < end; p.next = p.it.Next() {
			d.remove(p.next, &p.sym)
		}
	}

	for i := received; i < end; i++ {
		d.queue = append(d.queue, i)
	}
	if err := d.peel(); err != nil {
		d.err = err
		return err
	}

	// Rule (b): once the whole difference is recovered every symbol is
	// empty.
	if d.syms[0].isZero() && d.nonzero != 0 {
		d.err = fmt.Errorf("%w: symbol 0 empty while %d symbols are not", ErrCorrupt, d.nonzero)
		return d.err
	}
	return nil
}

// remove removes the coded symbol of a recovered id from symbol i and queues
// i if that may have made it pure.
func (d *Decoder) remove(i uint64, one *CodedSymbol) {
	s := &d.syms[i]
	if !s.isZero() {
		d.nonzero--
	}
	s.Sub(one)
	if !s.isZero() {
		d.nonzero++
	}
	if s.Count == 1 || s.Count == -1 {
		d.queue = append(d.queue, i)
	}
}

// peel recovers ids from pure symbols until the queue is empty. The queue is
// a stack, so a new batch is peeled from its highest index down: the sparse
// symbols first, and symbol 0, which holds the whole difference and so is the
// likeliest to look pure by a checksum collision, last.
func (d *Decoder) peel() error {
	end := d.Received()
	for len(d.queue) > 0 {
		j := d.queue[len(d.queue)-1]
		d.queue = d.queue[:len(d.queue)-1]

		s := &d.syms[j]
		if s.Count != 1 && s.Count != -1 {
			continue
		}
		id := s.Sum
		check := Checksum(id)
		if s.Check != check {
			continue
		}
		one := CodedSymbol{Sum: id, Check: check, Count: s.Count}

		// Rule (a): the id must map to j, or removing it would not clear j.
		it := d.m.Indices(id)
		idx := d.scratch[:0]
		next := it.Next()
		for ; next < end; next = it.Next() {
			idx = append(idx, next)
		}
		d.scratch = idx
		if _, ok := slices.BinarySearch(idx, j); !ok {
			d.queue = d.queue[:0]
			return fmt.Errorf("%w: id recovered at symbol %d does not map to it", ErrCorrupt, j)
		}
		if !d.r.ContainsID(id) {
			d.queue = d.queue[:0]
			return fmt.Errorf("%w: id recovered at symbol %d is outside range %s", ErrCorrupt, j, d.r)
		}
		if _, ok := d.seen[id]; ok {
			d.queue = d.queue[:0]
			return fmt.Errorf("%w: id recovered twice, at symbol %d", ErrCorrupt, j)
		}
		if uint64(d.Recovered()) >= peelFactor*end {
			d.queue = d.queue[:0]
			return fmt.Errorf("%w: more than %d peel steps for %d symbols", ErrCorrupt, peelFactor*end, end)
		}

		d.seen[id] = struct{}{}
		if one.Count == 1 {
			d.onlyLocal = append(d.onlyLocal, id)
		} else {
			d.onlyRem = append(d.onlyRem, id)
		}
		for _, i := range idx {
			d.remove(i, &one)
		}
		d.peeled = append(d.peeled, peeled{sym: one, it: it, next: next})
	}
	return nil
}

// Received returns the number of symbols added so far: the start of the next
// batch.
func (d *Decoder) Received() uint64 {
	return uint64(len(d.syms))
}

// Recovered returns the number of ids recovered so far, len(onlyLocal) +
// len(onlyRemote) of Result, without copying them.
func (d *Decoder) Recovered() int {
	return len(d.onlyLocal) + len(d.onlyRem)
}

// Done reports whether the difference is completely recovered: at least one
// symbol has been received, symbol 0 is empty and the decoder is not corrupt.
func (d *Decoder) Done() bool {
	return d.err == nil && len(d.syms) > 0 && d.syms[0].isZero()
}

// Result returns the ids recovered so far, each list sorted in ID.Compare
// order (ascending): onlyLocal are held by this node and not the peer,
// onlyRemote by the peer and not this node. Once Done, they are the whole
// difference. The slices are copies owned by the caller.
func (d *Decoder) Result() (onlyLocal, onlyRemote []ID) {
	onlyLocal = slices.Clone(d.onlyLocal)
	onlyRemote = slices.Clone(d.onlyRem)
	slices.SortFunc(onlyLocal, ID.Compare)
	slices.SortFunc(onlyRemote, ID.Compare)
	return onlyLocal, onlyRemote
}

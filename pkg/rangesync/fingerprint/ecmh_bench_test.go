// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

// This file holds a throwaway elliptic-curve multiset hash (ECMH) over
// secp256k1, used only to benchmark it against LtHash for open question 1 of
// the proposal (§14 Q1, §5.3). It is not a candidate implementation: the
// hash-to-curve is plain try-and-increment and nothing here is constant time.
// The result is recorded in the phase-1 Decisions log.

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"golang.org/x/crypto/sha3"
)

// ecmh is the multiset hash: the sum of the curve points of its elements. The
// zero value (Z == 0) is the point at infinity, the hash of the empty set.
type ecmh struct {
	p btcec.JacobianPoint
}

// ecmhPoint maps id to a curve point: the first x = keccak256("rangesync/ecmh"
// ‖ id ‖ ctr) that is on the curve, with even y.
func ecmhPoint(id fingerprint.ID, out *btcec.JacobianPoint) {
	h := sha3.NewLegacyKeccak256()
	var buf [len("rangesync/ecmh") + fingerprint.IDSize + 4]byte
	n := copy(buf[:], "rangesync/ecmh")
	n += copy(buf[n:], id[:])
	var x [32]byte
	for ctr := uint32(0); ; ctr++ {
		buf[n], buf[n+1], buf[n+2], buf[n+3] = byte(ctr>>24), byte(ctr>>16), byte(ctr>>8), byte(ctr)
		h.Reset()
		_, _ = h.Write(buf[:])
		h.Sum(x[:0])
		var fx, fy btcec.FieldVal
		if overflow := fx.SetByteSlice(x[:]); overflow {
			continue
		}
		if !btcec.DecompressY(&fx, false, &fy) {
			continue
		}
		*out = btcec.MakeJacobianPoint(&fx, &fy, new(btcec.FieldVal).SetInt(1))
		return
	}
}

func (e *ecmh) add(id fingerprint.ID) {
	var p btcec.JacobianPoint
	ecmhPoint(id, &p)
	btcec.AddNonConst(&e.p, &p, &e.p)
}

func (e *ecmh) sub(id fingerprint.ID) {
	var p btcec.JacobianPoint
	ecmhPoint(id, &p)
	p.Y.Negate(1).Normalize()
	btcec.AddNonConst(&e.p, &p, &e.p)
}

// addHash and subHash are the group operations a Fenwick node update or
// range query would run.
func (e *ecmh) addHash(o *ecmh) {
	btcec.AddNonConst(&e.p, &o.p, &e.p)
}

func (e *ecmh) subHash(o *ecmh) {
	n := o.p
	n.Y.Normalize().Negate(1).Normalize()
	btcec.AddNonConst(&e.p, &n, &e.p)
}

// digest is keccak256 of the compressed affine point (33 bytes).
func (e *ecmh) digest() [32]byte {
	a := e.p
	a.ToAffine()
	var b [33]byte
	b[0] = 2
	if a.Y.IsOdd() {
		b[0] = 3
	}
	a.X.PutBytesUnchecked(b[1:])
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(b[:])
	var d [32]byte
	h.Sum(d[:0])
	return d
}

// TestECMHGroup checks that the throwaway ECMH is a multiset hash, so that the
// benchmarks below measure a correct construction.
func TestECMHGroup(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(20))
	ids := make([]fingerprint.ID, 20)
	for i := range ids {
		ids[i] = randID(r)
	}
	var fwd, rev, empty ecmh
	for i := range ids {
		fwd.add(ids[i])
		rev.add(ids[len(ids)-1-i])
	}
	if fwd.digest() != rev.digest() {
		t.Fatal("insertion order changes the hash")
	}
	var half ecmh
	for _, id := range ids[:10] {
		half.add(id)
	}
	rest := fwd
	rest.subHash(&half)
	for _, id := range ids[10:] {
		rest.sub(id)
	}
	if rest.digest() != empty.digest() {
		t.Fatal("removing every element is not the empty hash")
	}
	if fwd.digest() == empty.digest() {
		t.Fatal("non-empty set hashes to the empty hash")
	}
	t.Logf("ECMH bucket: %d B in memory (Jacobian), 33 B compressed; LtHash bucket: %d B",
		reflect.TypeFor[ecmh]().Size(), reflect.TypeFor[fingerprint.LtHash]().Size())
}

func BenchmarkECMHAdd(b *testing.B) {
	r := rand.New(rand.NewSource(21))
	ids := make([]fingerprint.ID, 1024)
	for i := range ids {
		ids[i] = randID(r)
	}
	var e ecmh
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		e.add(ids[i%len(ids)])
		i++
	}
	b.ReportMetric(float64(reflect.TypeFor[ecmh]().Size()), "B/bucket")
}

func BenchmarkECMHAddHash(b *testing.B) {
	r := rand.New(rand.NewSource(22))
	var e, o ecmh
	e.add(randID(r))
	o.add(randID(r))
	b.ReportAllocs()
	for b.Loop() {
		e.addHash(&o)
	}
}

// BenchmarkECMHRange24 is the ECMH counterpart of BenchmarkLtHashRange24.
func BenchmarkECMHRange24(b *testing.B) {
	r := rand.New(rand.NewSource(23))
	nodes := make([]ecmh, 24)
	for i := range nodes {
		nodes[i].add(randID(r))
	}
	var out ecmh
	b.ReportAllocs()
	for b.Loop() {
		out = ecmh{}
		for i := range nodes {
			if i%2 == 0 {
				out.addHash(&nodes[i])
			} else {
				out.subHash(&nodes[i])
			}
		}
	}
}

func BenchmarkECMHDigest(b *testing.B) {
	r := rand.New(rand.NewSource(24))
	var e ecmh
	e.add(randID(r))
	e.add(randID(r))
	b.ReportAllocs()
	for b.Loop() {
		benchSink = e.digest()
	}
}

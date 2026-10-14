// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import "fmt"

// Group is the constraint on the elements of a Fenwick tree: a commutative
// group whose operations work in place on a pointer, so that large elements
// (a 2 KB LtHash, an array of coded symbols) are never copied. P is the pointer
// type *T; the methods must not retain their argument.
//
// Types whose own methods have other names or meanings (LtHash.Add adds an
// ID) are adapted by a defined type with the same underlying type, converted
// with a pointer conversion.
type Group[T any] interface {
	*T
	// Add sets the receiver to the receiver plus other.
	Add(other *T)
	// Sub sets the receiver to the receiver minus other.
	Sub(other *T)
	// Zero sets the receiver to the identity element.
	Zero()
}

// Fenwick is a Fenwick (binary indexed) tree over a flat slab of buckets of a
// group T: it keeps the sum of every bucket run in O(log n) stored nodes, so a
// bucket update and the sum of any run of consecutive buckets both take
// O(log n) group operations. Buckets are numbered 0..Len()-1 and runs are
// half-open, [i, j).
//
// Queries accumulate into a caller-provided element and do not allocate.
//
// Fenwick is not safe for concurrent use. Prefix and Range only read the
// tree, so they may run concurrently with each other, but not with Add, Sub,
// Update or Reset: every mutation needs exclusive access, and it writes the
// upper nodes that every query reads. The zero value is an empty tree with no
// buckets.
//
// The delta and out arguments must not point into the tree's own storage
// (the slab passed to NewFenwickFromBuckets).
type Fenwick[T any, P Group[T]] struct {
	// nodes[k-1] is the sum of the buckets [k-lowbit(k), k), where lowbit(k)
	// is the lowest set bit of k (the 1-indexed Fenwick layout).
	nodes []T
}

// NewFenwick returns a tree of n buckets, all set to the identity. It panics
// if n is negative.
func NewFenwick[T any, P Group[T]](n int) *Fenwick[T, P] {
	if n < 0 {
		panic(fmt.Sprintf("fingerprint: fenwick size %d is negative", n))
	}
	f := &Fenwick[T, P]{nodes: make([]T, n)}
	f.Reset()
	return f
}

// NewFenwickFromBuckets returns a tree whose bucket i holds buckets[i]. It
// builds the tree in O(n) group operations, in place: the tree takes
// ownership of buckets, which the caller must not use afterwards. This is the
// fast way to build a tree from scratch (fill plain buckets, then convert);
// n calls to Add take O(n log n).
func NewFenwickFromBuckets[T any, P Group[T]](buckets []T) *Fenwick[T, P] {
	n := len(buckets)
	for k := 1; k <= n; k++ {
		if parent := k + k&-k; parent <= n {
			P(&buckets[parent-1]).Add(&buckets[k-1])
		}
	}
	return &Fenwick[T, P]{nodes: buckets}
}

// Len returns the number of buckets.
func (f *Fenwick[T, P]) Len() int {
	return len(f.nodes)
}

// Add adds delta to bucket i. It panics unless 0 <= i < Len().
func (f *Fenwick[T, P]) Add(i int, delta *T) {
	f.checkBucket(i)
	for k := i + 1; k <= len(f.nodes); k += k & -k {
		P(&f.nodes[k-1]).Add(delta)
	}
}

// Sub subtracts delta from bucket i. It panics unless 0 <= i < Len().
func (f *Fenwick[T, P]) Sub(i int, delta *T) {
	f.checkBucket(i)
	for k := i + 1; k <= len(f.nodes); k += k & -k {
		P(&f.nodes[k-1]).Sub(delta)
	}
}

// Update applies a change to bucket i through fn, which is called once for
// each of the O(log n) stored nodes that cover the bucket. fn must apply the
// same group delta to every node it is given (for example add one element),
// otherwise the tree becomes inconsistent, and it must not retain the
// pointer. If fn panics, the nodes visited before it are already updated and
// the tree is inconsistent. Update is for elements where a delta is sparse and
// cheaper to apply directly than to materialize, such as one ID touching a few
// of a bucket's coded symbols. It panics unless 0 <= i < Len().
func (f *Fenwick[T, P]) Update(i int, fn func(node *T)) {
	f.checkBucket(i)
	for k := i + 1; k <= len(f.nodes); k += k & -k {
		fn(&f.nodes[k-1])
	}
}

// Prefix sets out to the sum of the buckets [0, j). It panics unless
// 0 <= j <= Len().
func (f *Fenwick[T, P]) Prefix(j int, out *T) {
	if j < 0 || j > len(f.nodes) {
		panic(fmt.Sprintf("fingerprint: fenwick prefix %d out of range [0, %d]", j, len(f.nodes)))
	}
	P(out).Zero()
	for ; j > 0; j &= j - 1 {
		P(out).Add(&f.nodes[j-1])
	}
}

// Range sets out to the sum of the buckets [i, j), prefix(j) − prefix(i). The
// nodes the two prefixes share are skipped, so a range reads at most
// popcount(i)+popcount(j) ≤ 2·bits.Len(n)−1 nodes. It panics unless
// 0 <= i <= j <= Len().
func (f *Fenwick[T, P]) Range(i, j int, out *T) {
	if i < 0 || i > j || j > len(f.nodes) {
		panic(fmt.Sprintf("fingerprint: fenwick range [%d, %d) out of range [0, %d]", i, j, len(f.nodes)))
	}
	P(out).Zero()
	// The node chains of i and j (clearing the lowest set bit each step) are
	// decreasing and meet at their largest common value; the nodes below it
	// cancel out.
	for i != j {
		if j > i {
			P(out).Add(&f.nodes[j-1])
			j &= j - 1
		} else {
			P(out).Sub(&f.nodes[i-1])
			i &= i - 1
		}
	}
}

// Reset sets every bucket to the identity.
func (f *Fenwick[T, P]) Reset() {
	for k := range f.nodes {
		P(&f.nodes[k]).Zero()
	}
}

func (f *Fenwick[T, P]) checkBucket(i int) {
	if i < 0 || i >= len(f.nodes) {
		panic(fmt.Sprintf("fingerprint: fenwick bucket %d out of range [0, %d)", i, len(f.nodes)))
	}
}

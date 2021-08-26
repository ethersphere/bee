package pot

import (
	"bytes"
	"encoding"
	"errors"
	"io"
)

const MaxDepth = 256

var (
	ErrNotFound = errors.New("not found")
)

type Entry interface {
	Key() []byte
	Equal(Entry) bool
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// Node is an interface for pot nodes
// implementations
type Node interface {
	Fork(po int) Node                               // Child node at PO po
	Append(CNode)                                   // append a CNode
	Iter(po int, f func(CNode) (bool, error)) error // iterate over children starting at PO po
	Pin(Entry)                                      // pin an entry to the node
	New() Node                                      // constructs a new Node
	Entry() Entry                                   // reconstructs the entry pinned to the Node
	// Size() int                                      // returns the number of entries under the node
	io.Closer
}

type Op func(Node, CNode, CNode) Node

type CNode struct {
	cur int
	Node
}

func NewCNode(n Node, i int) CNode {
	if n.Entry() == nil {
		n = nil
	}
	return CNode{i, n}
}

// Wedge
func Wedge(b Node, n, m CNode) Node {
	b.Pin(n.Entry())
	Append(b, n.Node, n.cur, m.cur)
	b.Append(m)
	Append(b, n.Node, m.cur+1, MaxDepth)
	return b
}

// Whirl
func Whirl(b Node, n, m CNode) Node {
	Append(b, n.Node, n.cur, m.cur)
	b.Append(CNode{m.cur, n.Node})
	b.Pin(m.Entry())
	return b
}

// Whack
func Whack(b Node, n, m CNode) Node {
	m = LastAt(n, m)
	Append(b, n.Node, n.cur, m.cur)
	b.Pin(m.Entry())
	Append(b, m.Node, m.cur+1, MaxDepth)
	return b
}

// Update
func Update(b Node, n CNode, k []byte, eqf func(Entry) Entry) Node {
	if n.Node == nil {
		b.Pin(eqf(nil))
		return b
	}
	if IsToPrune(n, k, eqf) {
		return b.New()
	}
	e := n.Entry()
	at := PO(e.Key(), k, n.cur)
	if at == MaxDepth {
		entry := eqf(e)
		if entry == nil {
			return Whack(b, n, CNode{MaxDepth, nil})
		}
		if entry.Equal(e) {
			return nil
		}
		return Whack(b, n, CNode{MaxDepth, Pin(b, entry)})
	}
	m := n.Fork(at)
	if m == nil {
		entry := eqf(nil)
		if entry == nil {
			return nil
		}
		return Whirl(b, n, CNode{at, Pin(b, entry)})
	}
	nn := CNode{at, m}
	if IsToPrune(nn, k, eqf) {
		return Whack(b, n, CNode{at, n})
	}
	return Update(Whirl(b, n, nn), nn, k, eqf)
}

func LastAt(n, m CNode) (k CNode) {
	if m.Node != nil {
		return m
	}
	_ = n.Iter(n.cur, func(c CNode) (bool, error) {
		k = c
		return k.cur < m.cur, nil
	})
	return k
}

func IsToPrune(n CNode, k []byte, eqf func(Entry) Entry) bool {
	if !Singleton(n.Node, n.cur) {
		return false
	}
	e := n.Entry()
	return eqf(e) == nil && bytes.Equal(e.Key(), k)
}

func Singleton(n Node, from int) (r bool) {
	_ = n.Iter(from, func(k CNode) (bool, error) {
		r = true
		return true, nil
	})
	return !r
}

func Append(b, n Node, from, to int) {
	_ = n.Iter(from, func(k CNode) (bool, error) {
		if k.cur < to {
			b.Append(k)
			return false, nil
		}
		return true, nil
	})
}

func Pin(n Node, e Entry) Node {
	k := n.New()
	k.Pin(e)
	return k
}

func Call(nodes []CNode, o []Op) {}

func Find(n Node, k []byte) (Entry, error) {
	return find(n, k, 0)
}

func find(n Node, k []byte, at int) (Entry, error) {
	if n == nil {
		return nil, ErrNotFound
	}
	e := n.Entry()
	if e == nil {
		return nil, ErrNotFound
	}
	if bytes.Equal(e.Key(), k) {
		return e, nil
	}
	at = PO(e.Key(), k, at)
	return find(n.Fork(at), k, at)
}

// po returns the proximity order of two fixed length byte sequences
// assuming po > pos
func PO(one, other []byte, pos int) int {
	for i := pos / 8; i < len(one); i++ {
		if one[i] == other[i] {
			continue
		}
		oxo := one[i] ^ other[i]
		start := 0
		if i == pos/8 {
			start = pos % 8
		}
		for j := start; j < 8; j++ {
			if (oxo>>uint8(7-j))&0x01 != 0 {
				return i*8 + j
			}
		}
	}
	return len(one) * 8
}

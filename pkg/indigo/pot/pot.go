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

type Val interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type Entry interface {
	Key() []byte
	Val() Val
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
	io.Closer
}

type Op func(Node, CNode, CNode) Node

type CNode struct {
	cur int
	Node
}

func NewCNode(n Node, i int) CNode {
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
	return b
}

// Whack
func Whack(b Node, n, m CNode) Node {
	Append(b, n.Node, n.cur, m.cur)
	b.Pin(m.Entry())
	Append(b, m.Node, m.cur+1, MaxDepth)
	return b
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

func Pin(n Node, e Entry) CNode {
	k := n.New()
	k.Pin(e)
	return CNode{MaxDepth, k}
}

func Call(nodes []CNode, o []Op) {}

func Find(n Node, k []byte) (Entry, error) {
	return find(n, k, 0)
}

func find(n Node, k []byte, at int) (Entry, error) {
	if n == nil {
		return nil, ErrNotFound
	}
	if bytes.Equal(n.Entry().Key(), k) {
		return n.Entry(), nil
	}
	at = PO(n.Entry().Key(), k, at)
	return find(n.Fork(at), k, at)
}

// func Add
func Update(b Node, n CNode, k []byte, eqf func(Entry) Entry) Node {
	at := PO(n.Entry().Key(), k, n.cur)
	if at == MaxDepth {
		entry := eqf(n.Entry())
		if entry == nil {
			return Whack(b, n, CNode{})
		}
		return Whack(b, n, Pin(b, entry))
	}
	m := n.Fork(at)
	if m == nil {
		return Whirl(b, n, Pin(b, eqf(nil)))
	}
	nn := CNode{at, m}
	return Update(Whirl(b, n, nn), nn, k, eqf)
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

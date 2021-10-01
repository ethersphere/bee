package pot

import (
	"encoding"
	"errors"
	"fmt"
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
	fmt.Stringer
}

// Node is an interface for pot nodes
// implementations
type Node interface {
	Fork(po int) CNode                                // Child node at PO po
	Append(CNode) Node                                // append a CNode
	Iter(from int, f func(CNode) (bool, error)) error // iterate over children starting at PO from
	Pin(Entry) Node                                   // pin an entry to the node
	Entry() Entry                                     // reconstructs the entry pinned to the Node
	// Size() int                                     // returns the number of entries under the node
}

type CNode struct {
	At   int
	Node Node
}

func NewCNode(n Node, i int) CNode {
	if n.Entry() == nil {
		n = nil
	}
	return CNode{i, n}
}

// Delete returns a delta for CNode c such that it instructs the Updater to remove the entry pinned to c's Node
// func (c CNode) Delete() Delta {
// 	return Delta{c.Node.Entry().Key(), func(_ Entry) Entry { return nil }}
// }

// Next returns a CNode, that is the view of the same Node from a po following the At of the receiver CNode
func (c CNode) Next() CNode {
	return CNode{c.At + 1, c.Node}
}

func isNil(c interface{}) bool {
	return c == nil
}

func Equal(a, b Entry) bool {
	return isNil(a) == isNil(b) && (isNil(a) || a.Equal(b))
}

func EntryOf(n Node) Entry {
	if n == nil {
		return nil
	}
	return n.Entry()
}

// Pin pins an entry to a node and returns the node
func Pin(n Node, e Entry) Node {
	n.Pin(e)
	return n
}

// Empty
func Empty(n Node) bool {
	return n == nil
}

func Append(b, n Node, from, to int) Node {
	_ = n.Iter(from, func(k CNode) (bool, error) {
		if k.At < to {
			b = b.Append(k)
			return false, nil
		}
		return true, nil
	})
	return b
}

func Find(n Node, k []byte) (Entry, error) {
	return find(CNode{0, n}, k)
}

func FindNext(n CNode, k []byte) (m CNode, b bool) {
	po, match := Compare(n.Node, k, n.At)
	if !match {
		m = n.Node.Fork(po)
	}
	return m, match
}

func find(n CNode, k []byte) (Entry, error) {
	if Empty(n.Node) || n.Node.Entry() == nil {
		return nil, ErrNotFound
	}
	m, match := FindNext(n, k)
	if match {
		return n.Node.Entry(), nil
	}
	return find(m, k)
}

func FindFork(n CNode, f func(CNode) bool) (m CNode) {
	_ = n.Node.Iter(n.At, func(c CNode) (bool, error) {
		m = c
		return f(m), nil
	})
	return m
}

// Compare compares the key of a CNode with a key, assuming the two match on a prefix of length po
// it returns the proximity order quantifying the distance of the two keys plus
// a boolean second return value which is true if the keys exactly match
func Compare(n Node, k []byte, at int) (po int, match bool) {
	po = PO(n.Entry().Key(), k, at)
	return po, po == MaxDepth
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

func Iter(n CNode, f func(Entry)) {
	f(n.Node.Entry())
	_ = n.Node.Iter(n.At, func(c CNode) (bool, error) {
		Iter(c.Next(), f)
		return false, nil
	})
}

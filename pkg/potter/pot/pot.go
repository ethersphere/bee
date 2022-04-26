package pot

import (
	"encoding"
	"encoding/binary"
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
	Pin(Entry)                                           // pin an entry to the node
	Entry() Entry                                        // get entry pinned to the Node
	Empty() bool                                         // get entry pinned to the Node
	Size() int                                           // get number of entries in the pot
	fmt.Stringer                                         // pretty print
	Fork(po int) CNode                                   // get child node fork at PO po
	Append(CNode)                                        // append child node to forks
	Truncate(i int)                                      // truncate fork list
	Iterate(from int, f func(CNode) (bool, error)) error // iterate over children starting at PO from
}

// CNode captures a child node (or cursor+node), a node viewed as a fork of a parent node
type CNode struct {
	At   int
	Node Node
	size int
}

// NewAt creates a cursored node and recalculates the size as the sum of sizes of its children
func NewAt(at int, n Node) CNode {
	if Empty(n) {
		// if n == nil || n.Size() == 0 {
		return CNode{at, nil, 0}
	}
	size := 1
	_ = n.Iterate(at+1, func(c CNode) (bool, error) {
		size += c.size
		return false, nil
	})
	return CNode{at, n, size}
}

// Next returns a CNode, that is the view of the same Node from a po following the At of the receiver CNode
func (c CNode) Next() CNode {
	return NewAt(c.At+1, c.Node)
}

// Size returns the number of entries (=Nodes) subsumed under the node
func (n CNode) Size() int {
	return n.size
}

func KeyOf(n Node) []byte {
	if Empty(n) {
		return nil
	}
	return n.Entry().Key()
}

func Label(k []byte) string {
	if len(k) == 0 {
		return "none"
	}
	return fmt.Sprintf("%08b", binary.BigEndian.Uint32(k[:4])>>24)
}

// Empty
func Empty(n Node) bool {
	return n == nil || n.Empty()
}

//
func Append(b, n Node, from, to int) {
	b.Truncate(from)
	_ = n.Iterate(from, func(k CNode) (bool, error) {
		if k.At < to {
			b.Append(k)
			return false, nil
		}
		return true, nil
	})
}

// Find finds the entry of a key
func Find(n Node, k []byte, mode Mode) (Entry, error) {
	return find(NewAt(-1, n), k, mode)
}

func find(n CNode, k []byte, mode Mode) (Entry, error) {
	if Empty(n.Node) {
		return nil, ErrNotFound
	}
	m, match := FindNext(n, k, mode)
	if match {
		return n.Node.Entry(), nil
	}
	return find(m, k, mode)
}

func ForAll(n Node, k []byte, mode Mode) (Node, int, error) {
	cn, err := findNode(NewAt(-1, n), k, mode)
	if err != nil {
		return nil, 0, err
	}
	return cn.Node, cn.size, nil
}

func findNode(n CNode, k []byte, mode Mode) (CNode, error) {
	if Empty(n.Node) {
		return CNode{}, ErrNotFound
	}
	if len(k) == n.At {
		return n, nil
	}
	m, _ := FindNext(n, k, mode)
	return findNode(m, k, mode)
}

// FindNext finds the fork on a node that matches the key bytes
func FindNext(n CNode, k []byte, mode Mode) (CNode, bool) {
	po := Compare(n.Node, k, n.At)
	if po < mode.Depth() {
		cn := n.Node.Fork(po)
		if err := mode.Unpack(cn.Node); err != nil {
			panic(err.Error())
		}
		return cn, false
	}
	return NewAt(MaxDepth, nil), true
}

// FindFork iterates through the forks on a node and returns the fork
// that returns true when fed to the stop function
// if no stop function is given then returns the last fork
func FindFork(n CNode, f func(CNode) bool, mode Mode) (m CNode) {
	_ = n.Node.Iterate(n.At, func(c CNode) (stop bool, err error) {
		if f == nil {
			m = c
		} else {
			stop = f(m)
		}
		return stop, nil
	})
	return m
}

// Compare compares the key of a CNode with a key, assuming the two match on a prefix of length po
// it returns the proximity order quantifying the distance of the two keys plus
// a boolean second return value which is true if the keys exactly match
func Compare(n Node, k []byte, at int) int {
	return PO(n.Entry().Key(), k, at)
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

// Iterate iterates through the entries of a pot top down depth first
func Iterate(n CNode, f func(Entry)) {
	f(n.Node.Entry())
	_ = n.Node.Iterate(n.At, func(c CNode) (bool, error) {
		Iterate(c.Next(), f)
		return false, nil
	})
}

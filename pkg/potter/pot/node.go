package pot

import "fmt"

// MemNode represents an in-memory (pointer-based) pot node
type MemNode struct {
	forks []CNode
	pin   Entry
}

var _ Node = (*MemNode)(nil)

// Fork returns child node corresponding to the fork at PO po
func (n *MemNode) Fork(po int) CNode {
	for _, cn := range n.forks {
		if cn.At == po {
			return cn
		}
		if cn.At > po {
			break
		}
	}
	return NewAt(po, nil)
}

// Append appends a CNode to the forks of MemNode n
func (n *MemNode) Append(cn CNode) {
	n.forks = append(n.forks, cn)
}

// Truncate chops forks at or after po given as arg
func (n *MemNode) Truncate(i int) {
	j := 0
	n.Iterate(0, func(cn CNode) (bool, error) {
		if cn.At >= i {
			return true, nil
		}
		j++
		return false, nil
	})
	if j < len(n.forks) {
		n.forks = n.forks[:j]
	}
}

// Iter iterates over children starting at PO po and applies f to them
// the iterator stops before completion if the function applied returns true (stop) or an error
func (n *MemNode) Iterate(po int, f func(CNode) (bool, error)) error {
	for _, cn := range n.forks {
		if cn.At >= po {
			if stop, err := f(cn); err != nil || stop {
				return err
			}
		}
	}
	return nil
}

// Pin an entry to the node
func (n *MemNode) Pin(e Entry) {
	n.pin = e
}

// Entry returns the entry pinned to the Node
func (n *MemNode) Entry() Entry {
	return n.pin
}

// Empty returns true if no entry is pinned to the Node
func (n *MemNode) Empty() bool {
	return n.pin == nil
}

// Size returns the number of entries (=Nodes) subsumed under the node
func (n *MemNode) Size() int {
	if n.Empty() {
		return 0
	}
	return NewAt(-1, n).Size()
}

var indent = "                                                                              "

// String pretty prints the pot
func (m *MemNode) String() string {
	return NewAt(-1, m).string(0)
}

func (n CNode) String() string {
	return fmt.Sprintf("POT %d (%d) - %s", n.At, n.Size(), n.string(0))
}

func (n CNode) string(i int) string {
	if i > 20 {
		return "..."
	}
	if n.Node == nil {
		return "nil"
	}
	j := 0
	s := fmt.Sprintf("K: %s, V: %s\n", Label(KeyOf(n.Node)), n.Node.Entry())
	n.Node.Iterate(n.At+1, func(c CNode) (bool, error) {
		s = fmt.Sprintf("%s%s> %d (%d) - %s", s, indent[:i], c.At, c.Size(), c.Next().string(i+1))
		j++
		return false, nil
	})

	return s
}

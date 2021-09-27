package pot

type MemNode struct {
	forks []CNode
	pin   Entry
}

var _ Node = (*MemNode)(nil)

// func NewMemNode(e Entry) *MemNode {
// 	return &MemNode{pin: e}
// }

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
	return CNode{}
}

func (n *MemNode) LastFork(from int) (cn CNode) {
	_ = n.Iter(from, func(c CNode) (bool, error) {
		cn = c
		return false, nil
	})
	return cn
}

// Append appends a CNode to the forks of MemNode n
func (n *MemNode) Append(cn CNode) {
	n.forks = append(n.forks, cn)
}

// Iter iterates over children starting at PO po and applies f to them
// the iterator before completion if the function applied returns true (stop) or an error
func (n *MemNode) Iter(po int, f func(CNode) (bool, error)) error {
	for _, cn := range n.forks {
		if cn.At >= po {
			if stop, err := f(cn); err != nil || stop {
				return err
			}
		}
	}
	return nil
}

// Close is noop for memory node
func (n *MemNode) Close() error {
	return nil
}

// pin an entry to the node
func (n *MemNode) Pin(e Entry) {
	n.pin = e
}

// constructs a new Node
func (n *MemNode) New() Node {
	return &MemNode{}
}

// reconstructs the entry pinned to the Node
func (n *MemNode) Entry() Entry {
	return n.pin
}

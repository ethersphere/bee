package pot

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
	return CNode{}
}

// Append appends a CNode to the forks of MemNode n
func (n *MemNode) Append(cn CNode) Node {
	n.forks = append(n.forks, cn)
	return n
}

// Iter iterates over children starting at PO po and applies f to them
// the iterator stops before completion if the function applied returns true (stop) or an error
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

// pin an entry to the node
func (n *MemNode) Pin(e Entry) Node {
	n.pin = e
	return n
}

// reconstructs the entry pinned to the Node
func (n *MemNode) Entry() Entry {
	return n.pin
}

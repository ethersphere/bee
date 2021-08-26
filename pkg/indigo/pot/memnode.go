package pot

type MemNode struct {
	forks []CNode
	pin   Entry
}

// Child node at PO po
func (n *MemNode) Fork(po int) Node {
	for _, cn := range n.forks {
		if cn.cur == po {
			return cn.Node
		}
		if cn.cur > po {
			break
		}
	}
	return nil
}

// append a CNode
func (n *MemNode) Append(cn CNode) {
	n.forks = append(n.forks, cn)
}

// iterate over children starting at PO po
func (n *MemNode) Iter(po int, f func(CNode) (bool, error)) error {
	for _, cn := range n.forks {
		if cn.cur >= po {
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

package pot

import (
	"context"

	"github.com/ethersphere/bee/pkg/indigo/persister"
)

var _ Node = (*DBNode)(nil)
var _ persister.TreeNode = (*DBNode)(nil)

type DBNode struct {
	ls   persister.LoadSaver
	ref  []byte
	node Node
}

func NewDBNode(ref []byte, ls persister.LoadSaver) *DBNode {
	return &DBNode{ref: ref, ls: ls}
}

// Close is noop for memory node
func (n *DBNode) Close() error {
	return n.ls.Close()
}

// Child node at PO po
func (n *DBNode) Fork(po int) Node {
	return n.Node().Fork(po)
}

// append a CNode
func (n *DBNode) Append(cn CNode) {
	n.Node().Append(cn)
}

// iterate over children starting at PO po
func (n *DBNode) Iter(po int, f func(CNode) (bool, error)) error {
	return n.Node().Iter(po, f)
}

// pin an entry to the node
func (n *DBNode) Pin(e Entry) {
	n.Node().Pin(e)
}

// constructs a new Node
func (n *DBNode) New() Node {
	return &DBNode{ls: n.ls, node: (&MemNode{}).New()}
}

// reconstructs the entry pinned to the Node
func (n *DBNode) Entry() Entry {
	return n.Node().Entry()
}

// reconstructs the entry pinned to the Node
func (n *DBNode) Node() Node {
	if n.node == nil {
		persister.Load(context.Background(), n, n.ls)
	}
	return n.node
}

func (n *DBNode) Reference() []byte {
	if len(n.ref) == 0 {
		persister.Save(context.Background(), n, n.ls)
	}
	return n.ref
}

func (n *DBNode) SetReference(ref []byte) {
	n.ref = ref
}

func (n *DBNode) Children(f func(persister.TreeNode)) {
	// n.Node().Iter(0, func(cn CNode) (stop bool, err error) {
	// 	f(cn.Node.(*DBNode))
	// })
}

func (n *DBNode) MarshalBinary() ([]byte, error) {
	return nil, nil
}

func (n *DBNode) UnmarshalBinary([]byte) error {
	return nil
}

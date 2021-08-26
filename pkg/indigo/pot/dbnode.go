package pot

import (
	"context"
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/indigo/persister"
)

var _ Node = (*DBNode)(nil)
var _ persister.TreeNode = (*DBNode)(nil)

type DBNode struct {
	ls     persister.LoadSaver
	entryf func() Entry
	ref    []byte
	node   *MemNode
}

// func NewDBNode(ref []byte, ls persister.LoadSaver) *DBNode {
// 	return &DBNode{ref: ref, ls: ls, node: &MemNode{}}
// }

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
func NewDBNode(ls persister.LoadSaver, entryf func() Entry) *DBNode {
	return &DBNode{ls: ls, entryf: entryf}
}

// constructs a new Node
func (n *DBNode) New() Node {
	return NewDBNode(n.ls, n.entryf)
}

// reconstructs the entry pinned to the Node
func (n *DBNode) Entry() Entry {
	return n.Node().Entry()
}

// reconstructs the entry pinned to the Node
func (n *DBNode) Node() *MemNode {
	if n.node == nil {
		if len(n.ref) == 0 {
			n.node = &MemNode{}
			return n.node
		}
		persister.Load(context.Background(), n)
	}
	return n.node
}

func (n *DBNode) Reference() []byte {
	return n.ref
}

func (n *DBNode) SetReference(ref []byte) {
	n.ref = ref
}

func (n *DBNode) LoadSaver() persister.LoadSaver {
	return n.ls
}

func (n *DBNode) MarshalBinary() ([]byte, error) {
	entry, err := n.Entry().MarshalBinary()
	if err != nil {
		return nil, err
	}
	l := len(entry)
	buf := make([]byte, l+4)
	binary.BigEndian.PutUint32(buf, uint32(l))
	copy(buf[4:], entry)
	ctx := context.Background()
	err = n.Node().Iter(0, func(cn CNode) (bool, error) {
		buf = append(buf, uint8(cn.cur))
		ref, err := persister.Reference(ctx, cn.Node.(persister.TreeNode))
		if err != nil {
			return true, err
		}
		buf = append(buf, ref...)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (n *DBNode) UnmarshalBinary(buf []byte) error {
	l := binary.BigEndian.Uint32(buf[:4])
	e := n.entryf()
	if err := e.UnmarshalBinary(buf[4 : 4+l]); err != nil {
		return err
	}
	n.node.pin = e
	for i := 4 + l; i < uint32(len(buf)); i += 10 {
		m := n.New()
		m.(persister.TreeNode).SetReference(buf[i+1 : i+10])
		cn := CNode{cur: int(uint8(buf[i])), Node: m}
		n.node.Append(cn)
	}
	return nil
}

func SetReference(n persister.TreeNode, ref []byte) {
	n.SetReference(ref)
}

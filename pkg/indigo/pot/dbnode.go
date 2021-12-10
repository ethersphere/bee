package pot

import (
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/indigo/persister"
)

var _ Node = (*DBNode)(nil)
var _ persister.TreeNode = (*DBNode)(nil)

type DBNode struct {
	ref []byte
	*MemNode
	entryf func() Entry
}

func (n *DBNode) Reference() []byte {
	return n.ref
}

func (n *DBNode) SetReference(ref []byte) {
	n.ref = ref
}

// iterate over children
func (n *DBNode) Children(f func(persister.TreeNode) error) error {
	g := func(cn CNode) (bool, error) {
		return false, f(cn.Node.(*DBNode))
	}
	return n.Iter(0, g)
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
	err = n.Iter(0, func(cn CNode) (bool, error) {
		buf = append(buf, uint8(cn.At))
		buf = append(buf, cn.Node.(*DBNode).Reference()...)
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
	n.Pin(e)
	for i := 4 + l; i < uint32(len(buf)); i += 10 {
		m := &DBNode{ref: buf[i+1 : i+10], entryf: n.entryf}
		cn := CNode{int(uint8(buf[i])), m}
		n.Append(cn)
	}
	return nil
}

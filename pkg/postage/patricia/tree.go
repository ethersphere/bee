package patricia

import (
	"bytes"
)

const size = 256

type Node struct {
	Val      []byte
	Children map[byte]*Node // length 256 - fork on every byte
}

func NewNode(v []byte) *Node {
	n := &Node{
		Val: v,
	}
	if v == nil {
		n.Children = make(map[byte]*Node)
	}
	return n
}

func (n *Node) isLeaf() bool {
	if n.Val == nil {
		return false
	}
	return true
}

func (n *Node) Insert(nn *Node) (inserted bool) {
	idx := nn.Val[0] // take the first byte
	switch v := n.Children[idx]; v {
	case nil:
		// child doesn't exist, put the whole thing there
		n.Children[idx] = nn
		return true
	default:
		// if we are a leaf - split into two
		if v.isLeaf() {
			if bytes.Equal(v.Val, nn.Val) {
				return false
			}
			newNode := NewNode(nil)

			newChild := NewNode(v.Val[1:])
			newNode.Insert(newChild)
			newVal := nn.Val[1:] // remove first byte and insert into the child
			nn.Val = newVal
			ret := newNode.Insert(nn)
			n.Children[idx] = newNode
			return ret
		} else {
			// we're not a leaf - call the insert on the child
			newVal := nn.Val[1:] // remove first byte and insert into the child
			nn.Val = newVal
			return n.Children[idx].Insert(nn)
		}
	}
}

//func (n *Node) MarshalBinary() ([]byte, error) {
//valLen := uint16(len(n.Val))
//res := make([]byte, valLen+2)
//binary.BigEndian.PutUint16(res, valLen)
//copy(res[2:], n.Val)

//if n.children == nil {
//return res, nil
//}

//for _, nn := range n.children {
//child, err := nn.MarshalBinary()
//if err != nil {
//return nil, err
//}
//res = append(res, child...)
//}

//return res, nil

//}

package patricia

import (
	"bytes"
	"fmt"
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
	//if v == nil {
	n.Children = make(map[byte]*Node)
	//}
	return n
}

func (n *Node) isLeaf() bool {
	if n.Val == nil {
		return false
	}
	return true
}

func (n *Node) Insert(nn *Node) (inserted bool) {
	fmt.Println("insert", n.Val, nn.Val)
	idx := nn.Val[0] // take the first byte
	switch v := n.Children[idx]; v {
	case nil:
		// child doesn't exist, put the whole thing there
		//nn.Val = nn.Val[1:]

		n.Children[idx] = nn
		return true
	default:
		// if we are a leaf - split into two
		if v.isLeaf() {
			if bytes.Equal(v.Val, nn.Val) {
				return false
			}

			newNode := NewNode(nil)

			fmt.Println("val", v.Val)

			// note: this is wasteful, we can mutate `v` here
			newChild := NewNode(v.Val[1:])
			fmt.Println("node val", newChild.Val)

			newNode.Insert(newChild)

			fmt.Println("new val", nn.Val, nn.Val[1:])
			nn.Val = nn.Val[1:] // remove first byte and insert into the child
			ret := newNode.Insert(nn)
			n.Children[idx] = newNode
			return ret
		}
		// we're not a leaf - call the insert on the child
		newVal := nn.Val[1:] // remove first byte and insert into the child
		nn.Val = newVal
		return n.Children[idx].Insert(nn)
	}
}

func (n *Node) MarshalBinary() ([]byte, error) {
	b := make([]byte, len(n.Val)+1)
	b[0] = uint8(len(n.Val))
	copy(b[1:], n.Val)

	// the byte value, then we need to serialize the node

	for k, v := range n.Children {
		bb := make([]byte, 1)
		bb[0] = k

		output, _ := v.MarshalBinary()

		bb = append(bb, output...)
		b = append(b, bb...)
	}

	return b, nil
}

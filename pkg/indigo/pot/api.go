package pot

func Add(root Node, e Entry) Node {
	Update(root.New(), NewCNode(root, 0), Delta{e.Key(), func(_ Entry) Entry { return e }})
}

func Delete(root Node, k []byte) Node {
	Update(root.New(), NewCNode(root, 0), Delta{k, func(_ Entry) Entry { return nil }})
}


func update(next Step, acc Node, n CNode) Node {
	m := next.Node()
	op, stop := nextF(next)
	switch op := next.Op();op  {
	case Wedge:
		return Wedge(acc, n, update(next.Next(), acc.New(), m))
	case Whirl, Whack_:
		return update(next.Next(), op(acc, n, m), m)
	default:
		return nil
	}
}

func Update(b Node, n CNode, delta Delta) Node {
	update(acc Node, n CNode, step Step, k []byte, delta Delta, po int, match bool) Node 
	
}

func Next(s Step) Op {
	switch {
	case change.Start():
		case change.None():
			return nil
		case m == nil :
			change.Terminal():			
			m = 
		case change.Deleted():
			acc = Whirl(acc, n, m)
		default:
		}
	}


	po, match = Compare(n.Node, k, n.At)				
	m := n
	var orig Entry
	if !match {
		m = n.Fork(po)
		if !Empty(m) {
			orig = m.Node.Entry()
		}
	}
	change := 
	return Update(acc, n, change, k, delta.Apply(orig))
}



// Update
// func Update(b Node, n CNode, delta Delta) Node {
// 	if Empty(n.Node) {
// 		return Pin(b, delta.Apply(nil).Val)
// 	}
// 	po, match := Compare(n.Node, delta.Key, n.At-1)
// 	if match {
// 		change := delta.Apply(n.Node.Entry())
// 		switch {
// 		case change.Deleted:
// 			last := n.Node.LastFork(n.At)
// 			if last.Node == nil { // is singleton
// 				return b
// 			}
// 			return Update(Whack(b, n, last), last.Next(), last.Delete())

// 		case change.None:
// 			return nil
// 		default:
// 			return Whack(b, n, CNode{po, Pin(b.New(), change.Entry)})
// 		}
// 	}
// 	m := n.Node.Fork(po)
// 	if Empty(m.Node) {
// 		change := delta.Apply(nil)
// 		if change.Deleted {
// 			return nil
// 		}
// 		return Whirl(b, n, CNode{po, Pin(b.New(), change.Entry)})
// 	}
// 	return Update(Whirl(b, n, m), m.Next(), delta)
// }


// weave
func weave(acc Node, n CNode, stop func(CNode) bool) Step {
	AppendTill(acc, n, n.At, )
}



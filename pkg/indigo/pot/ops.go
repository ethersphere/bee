package pot

// Wedge
func Wedge(acc Node, n, m CNode) Node {
	acc = Append(acc, n.Node, n.At, m.At)
	if !Empty(m.Node) {
		acc = acc.Append(m)
	}
	acc = Append(acc, n.Node, m.At+1, MaxDepth)
	return acc.Pin(n.Node.Entry())
}

// Whirl
func Whirl(acc Node, n, m CNode) Node {
	acc = Append(acc, n.Node, n.At, m.At)
	acc = acc.Append(CNode{m.At, n.Node})
	return acc.Pin(m.Node.Entry())
}

// Whack
func Whack(acc Node, n, m CNode) Node {
	acc = Append(acc, n.Node, n.At, m.At)
	acc = acc.Append(CNode{m.At, n.Node})
	acc = Append(acc, m.Node, m.At+1, MaxDepth)
	return acc.Pin(m.Node.Entry())
}

// Update
func Update(acc Node, cn CNode, k []byte, eqf func(Entry) Entry, mode Mode) Node {
	return mode.Pack(update(acc, cn, k, eqf, mode))
}

func update(acc Node, cn CNode, k []byte, eqf func(Entry) Entry, mode Mode) Node {
	if Empty(cn.Node) {
		e := eqf(nil)
		if e == nil {
			return nil
		}
		return acc.Pin(e)
	}
	cm, match := FindNext(cn, k)
	if match {
		orig := cn.Node.Entry()
		entry := eqf(orig)
		if entry == nil {
			return Pull(acc, cn, mode)
		}
		if entry.Equal(orig) {
			return nil
		}
		return Whack(acc, cn, CNode{MaxDepth, mode.New().Pin(entry)})
	}
	if Empty(cm.Node) {
		entry := eqf(nil)
		if entry == nil {
			return nil
		}
		return Whirl(acc, cn, CNode{cm.At, mode.New().Pin(entry)})
	}
	if cm.At == 0 {
		cm := CNode{0, update(acc, cm, k, eqf, mode)}
		if cm.Node == nil {
			return nil
		}
		if mode.Down(cm) {
			return Wedge(mode.New(), cn, cm)
		}
		res := Wedge(mode.New(), cm, cn)
		return res
	}
	if mode.Down(cm) {
		res := update(mode.New(), cm, k, eqf, mode)
		if res == nil {
			return nil
		}
		return Wedge(acc, cn, CNode{cm.At, res})
	}
	return update(Whirl(acc, cn, cm), cm.Next(), k, eqf, mode)
}

func Pull(acc Node, cn CNode, mode Mode) Node {
	if f := mode.Up(); f == nil {
		cm := FindFork(cn, nil)
		if !Empty(cm.Node) {
			return pullTail(Wedge(acc, cn, CNode{cm.At, nil}), cm.Next(), mode)
		}
		j := cn.At - 1
		cn = acc.Fork(j)
		acc.Truncate(j)
		if cn.Node == nil {
			// this happens only if the pot is singleton
			return mode.New()
		}
		return Wedge(acc, cn, CNode{j, nil})
	}
	return pull(acc, cn, mode)
}

func pull(acc Node, cn CNode, mode Mode) Node {
	return nil
}

func pullTail(acc Node, cn CNode, mode Mode) Node {
	cm := FindFork(cn, nil)
	if Empty(cm.Node) {
		return Wedge(acc, cn, CNode{MaxDepth, nil})
	}
	return pullTail(Whirl(acc, cn, cm), cm.Next(), mode)
}

// res := pull(mode.New(), cn, mode)
// 	if !Empty(res) {
// 		return Whack(acc, cn, CNode{cn.At, res})
// 	}

// 	fmt.Println("pulling", cn)
// 	cm := FindFork(cn, mode.Up)
// 	if Empty(cm.Node) {
// 		// fmt.Println("none", cn)
// 		return nil
// 	}
// 	res := Pull(mode.New(), cm.Next(), mode)
// 	if res == nil {
// 		fmt.Println("whack", cn)
// 		return mode.New().Pin(cm.Node.Entry())
// 		// return Whack(acc, cn, CNode{MaxDepth, mode.New().Pin(cm.Node.Entry())})
// 	}
// 	res = Wedge(acc, cn, CNode{cm.At, res}).Pin
// 	fmt.Println("pulled", res)
// 	return res
// }

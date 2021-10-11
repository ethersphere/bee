package pot

// Wedge
func Wedge(acc Node, n, m CNode) Node {
	acc = Append(acc, n.Node, n.At, m.At)
	acc = acc.Append(m).Pin(n.Node.Entry())
	return Append(acc, n.Node, m.At+1, MaxDepth)
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
			res := Pull(acc, cn, mode)
			if Empty(res) {
				return mode.New()
			}
			return res
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
	if mode.Down(cm) {
		res := Update(mode.New(), cm, k, eqf, mode)
		if res == nil {
			return nil
		}
		return Wedge(acc, cn, CNode{cm.At, res})
	}
	res := update(Whirl(acc, cn, cm), cm.Next(), k, eqf, mode)
	if res == nil {
		return nil
	}
	if res.Entry() == nil {
		acc = acc.Append(CNode{cm.At, nil})
		return acc.Pin(cn.Node.Entry())
	}
	return res
}

func Pull(acc Node, cn CNode, mode Mode) Node {
	cm := FindFork(cn, mode.Up)
	if Empty(cm.Node) {
		return nil
	}
	res := Pull(mode.New(), cm.Next(), mode)
	if res == nil {
		return acc.Pin(cm.Node.Entry())
	}
	return Whack(acc, CNode{cm.At, res}, cn.Next())
}

package pot

// Wedge
func Wedge(acc Node, n, m CNode) {
	Append(acc, n.Node, n.At, m.At)
	if !Empty(m.Node) {
		acc.Append(m)
	}
	Append(acc, n.Node, m.At+1, MaxDepth)
	acc.Pin(n.Node.Entry())
}

// Whirl
func Whirl(acc Node, n, m CNode) {
	Append(acc, n.Node, n.At, m.At)
	acc.Append(NewAt(m.At, n.Node))
	acc.Pin(m.Node.Entry())
}

// Whack
func Whack(acc Node, n, m CNode) {
	Append(acc, n.Node, n.At, m.At)
	if m.At < MaxDepth {
		acc.Append(NewAt(m.At, n.Node))
	}
	Append(acc, m.Node, m.At+1, MaxDepth)
	acc.Pin(m.Node.Entry())
}

// Update
func Update(acc Node, cn CNode, k []byte, eqf func(Entry) Entry, mode Mode) Node {
	u := update(acc, cn, k, eqf, mode)
	if err := mode.Pack(u); err != nil {
		panic(err.Error())
	}
	return u
}

func update(acc Node, cn CNode, k []byte, eqf func(Entry) Entry, mode Mode) Node {
	if Empty(cn.Node) {
		e := eqf(nil)
		if e == nil {
			return nil
		}
		acc.Pin(e)
		return acc
	}
	cm, match := FindNext(cn, k, mode)
	if match {
		orig := cn.Node.Entry()
		entry := eqf(orig)
		if entry == nil {
			return Pull(acc, cn, mode)
		}
		if entry.Equal(orig) {
			return nil
		}
		n := mode.New()
		n.Pin(entry)
		Whack(acc, cn, NewAt(mode.Depth(), n))
		return acc
	}
	if Empty(cm.Node) {
		entry := eqf(nil)
		if entry == nil {
			return nil
		}
		n := mode.New()
		n.Pin(entry)
		Whirl(acc, cn, NewAt(cm.At, n))
		return acc
	}
	if cm.At == 0 {
		res := update(acc, cm, k, eqf, mode)
		cm := NewAt(-1, res)
		if cm.Node == nil {
			Wedge(acc, cn, NewAt(0, cm.Node))
			return acc
		}
		if mode.Down(cm) {
			acc := mode.New()
			Wedge(acc, cn, cm)
			return acc
		}
		n := mode.New()
		Whack(n, cm, cn)
		return n
	}
	if mode.Down(cm) {
		res := update(mode.New(), cm, k, eqf, mode)
		if res == nil {
			return nil
		}
		Wedge(acc, cn, NewAt(cm.At, res))
		return acc
	}
	Whirl(acc, cn, cm)
	return update(acc, cm.Next(), k, eqf, mode)
}

func Pull(acc Node, cn CNode, mode Mode) Node {
	if f := mode.Up(); f == nil {
		cm := FindFork(cn, nil, mode)
		if !Empty(cm.Node) {
			Wedge(acc, cn, NewAt(cm.At, nil))
			return pullTail(acc, cm.Next(), mode)
		}
		j := cn.At - 1
		cn = acc.Fork(j)
		acc.Truncate(j)
		if cn.Node == nil {
			// this happens only if the pot is singleton
			return mode.New()
		}
		Wedge(acc, cn, NewAt(j, nil))
		return acc
	}
	return pull(acc, cn, mode)
}

func pull(acc Node, cn CNode, mode Mode) Node {
	return nil
}

func pullTail(acc Node, cn CNode, mode Mode) Node {
	cm := FindFork(cn, nil, mode)
	if Empty(cm.Node) {
		Wedge(acc, cn, NewAt(mode.Depth(), nil))
		return acc
	}
	Whirl(acc, cn, cm)
	return pullTail(acc, cm.Next(), mode)
}

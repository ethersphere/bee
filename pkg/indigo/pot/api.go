package pot

func Add(root Node, e Entry, mode Mode) Node {
	return Update(mode.New(), NewCNode(root, 0), Step{
		Key:    e.Key(),
		Update: func(_ Entry) Entry { return e },
	}, mode)
}

func Delete(root Node, k []byte, mode Mode) Node {
	return Update(mode.New(), NewCNode(root, 0), Step{
		Key:    k,
		Update: func(_ Entry) Entry { return nil },
	}, mode)
}

type SingleOrder struct{}

var _ Mode = (*SingleOrder)(nil)

func (_ SingleOrder) Pack(n Node) Node {
	return n
}

func (_ SingleOrder) Unpack(n Node) Node {
	return n
}

func (_ SingleOrder) Down(_ CNode) bool {
	return false
}

func (_ SingleOrder) Up(_ CNode) bool {
	return false
}

// constructs a new Node
func (_ SingleOrder) New() Node {
	return &MemNode{}
}

// Update
func Update(acc Node, cn CNode, step Step, mode Mode) Node {
	cm := step.CNode
	if Empty(cn.Node) {
		cn.Node = mode.New()
	}
	if step.Match() {
		if step.Unchanged() {
			return nil
		}
		if step.Deletion() {
			return Whack(acc, cn, cm)
		}
		if Empty(cm.Node) {
		}
		return Pin(acc, step.After)
	}
	if Empty(cm.Node) {
		if step.Deletion() {
			return nil
		}
		return Whirl(acc, cn, cm)
	}

	nextStep := step.Next(mode)
	if step.Deletion() || !mode.Down(cm) {
		return mode.Pack(Update(Whirl(acc, cn, cm), cm.Next(), nextStep, mode))
	}
	return Wedge(acc, cn, CNode{cm.At, mode.Pack(Update(mode.New(), cm, nextStep, mode))})

}

package pot

// type Delta struct {
// 	Key []byte
// 	F   func(Entry) Entry
// }

// // Apply applies the function to the entry
// func (delta Delta) Apply(po int, n Node) Step {
// 	e := EntryOf(n)
// 	return Step{e, delta.F(e), po, n}
// }

// type Op int

// const (
// 	Whirl_ Op = iota
// 	Whack_
// 	Wedge_
// )

type Step struct {
	Mode   Mode
	Key    []byte
	Update func(Entry) Entry
	After  Entry
	Before Entry
	CNode  CNode
}

func (s Step) Unchanged() bool {
	return Equal(s.Before, s.After)
}

func (s Step) Match() bool {
	return s.CNode.At == MaxDepth
}

func (s Step) Deletion() bool {
	return s.After == nil
}

func (s Step) NextCNode(mode Mode) CNode {
	if s.Deletion() {
		return FindFork(s.CNode, mode.Up)
	}
	cm, _ := FindNext(s.CNode, s.Key)
	return cm
}

func (s Step) Next(mode Mode) Step {
	cn := s.NextCNode(mode)
	mode.Unpack(cn.Node)
	ns := Step{
		CNode: cn,
		Key:   s.Key,
	}
	if ns.Match() {
		ns.Before = cn.Node.Entry()
		ns.After = s.Update(ns.Before)
		ns.Update = s.Update
	}
	return ns
}

// Wedge
func Wedge(acc Node, n, m CNode) Node {
	return acc.Append(m).Pin(n.Node.Entry())
}

// Whirl
func Whirl(acc Node, n, m CNode) Node {
	return acc.Append(CNode{m.At, n.Node}).Pin(m.Node.Entry())
}

// Whack
func Whack(acc Node, n, m CNode) Node {
	return acc.Pin(m.Node.Entry())
}

type Mode interface {
	New() Node // consructor
	Pack(Node) Node
	Unpack(Node) Node
	Down(CNode) bool // to determine which entry should be promoted
	Up(CNode) bool   // to determine which node/entry to promote after deletion
}

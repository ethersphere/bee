package pot

type Delta struct {
	Key []byte
	F   func(Entry) Entry
}

// Apply applies the function to the entry
func (delta Delta) Apply(po int, n Node) Step {
	e := EntryOf(n)
	return Step{e, delta.F(e), po, n}
}

type Op int

const (
	Whirl_ Op = iota
	Whack_
	Wedge_
)

type Step struct {
	Key    []byte
	F      func(Entry) Entry
	After  Entry
	Before Entry
	PO     int
	Node   Node
}

func (s Step) None() bool {
	return Equal(s.Before, s.After)
}

func (s Step) Match() bool {
	return s.PO == MaxDepth
}

func (s Step) Deleted() bool {
	return s.After == nil
}

func (s Step) Next(n Node) CNode {
	if s.Node == nil {
		return CNode{s.PO, Pin(n.New(), s.After)}
	}
	return CNode{s.PO, s.Node}

}

func AppendTill(acc Node, n CNode, stop func(CNode) bool) {
	_ = n.Node.Iter(n.At, func(k CNode) (bool, error) {
		if !stop(k) {
			acc.Append(k)
			return false, nil
		}
		return true, nil
	})
}

// Wedge
func Wedge(acc Node, n, m CNode) Node {
	acc.Pin(n.Node.Entry())
	acc.Append(m)
	return acc
}

// Whirl
func Whirl(acc Node, n, m CNode) Node {
	acc.Append(CNode{m.At, n.Node})
	acc.Pin(m.Node.Entry())
	return acc
}

// Whack
func Whack(acc Node, n, m CNode) Node {
	acc.Append(CNode{m.At, Pin(acc.New(), m.Node.Entry())})
	return acc
}

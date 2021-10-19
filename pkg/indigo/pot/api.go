package pot

import (
	"context"

	"github.com/ethersphere/bee/pkg/indigo/persister"
)

func Add(root Node, e Entry, mode Mode) Node {
	return Update(mode.New(), CNode{0, root}, e.Key(), func(Entry) Entry { return e }, mode)
}

func Delete(root Node, k []byte, mode Mode) Node {
	return Update(mode.New(), CNode{0, root}, k, func(Entry) Entry { return nil }, mode)
}

type Mode interface {
	New() Node // consructor
	Pack(Node) Node
	Unpack(Node) Node
	Down(CNode) bool      // to determine which entry should be promoted
	Up() func(CNode) bool // to determine which node/entry to promote after deletion
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

func (_ SingleOrder) Up() func(CNode) bool {
	return nil
}

// constructs a new Node
func (_ SingleOrder) New() Node {
	return &MemNode{}
}

type PersistedPot struct {
	Mode
	ls     persister.LoadSaver
	entryf func() Entry
}

// constructs a new Node
func NewPersistedPot(mode Mode, entryf func() Entry) *PersistedPot {
	return &PersistedPot{Mode: mode, entryf: entryf}
}

func (pm *PersistedPot) NewFromReference(ref []byte) *DBNode {
	return &DBNode{entryf: pm.entryf, ref: ref, MemNode: &MemNode{}}
}

func (pm *PersistedPot) Pack(n Node) Node {
	_, _ = persister.Save(context.Background(), pm.ls, n.(persister.TreeNode))
	return n
}

func (pm *PersistedPot) Unpack(n Node) Node {
	tn := n.(persister.TreeNode)
	if len(tn.Reference()) > 0 {
		_ = persister.Load(context.Background(), pm.ls, tn)
	}
	return n
}

// constructs a new Node
func (pm *PersistedPot) New() Node {
	return &DBNode{entryf: pm.entryf}
}

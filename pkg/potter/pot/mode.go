package pot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/ethersphere/bee/pkg/potter/persister"
)

type Mode interface {
	Depth() int                                           // maximum bit length of key
	New() Node                                            // constructor
	Pack(Node) error                                      // mode specific saveing of a node
	Unpack(Node) error                                    // mode specific loading of a node
	Down(CNode) bool                                      // dictates insertion policy
	Up() func(CNode) bool                                 // dictates which node/entry to promote after deletion
	Load() (Node, bool, error)                            // loads the pot
	Save(_ []byte) error                                  // saves the pot
	Update(Node, []byte, func(Entry) Entry) (Node, error) // mode specific update
	io.Closer                                             // Close closes open files etc.
}

type SingleOrder struct {
	depth int
}

var _ Mode = (*SingleOrder)(nil)

func NewSingleOrder(d int) *SingleOrder {
	return &SingleOrder{depth: d}
}

// Pack NOOP
func (_ SingleOrder) Pack(n Node) error {
	return nil
}

// Unpack NOOP
func (_ SingleOrder) Unpack(n Node) error {
	return nil
}

// Down dictates insert policy - NOOP
func (_ SingleOrder) Down(_ CNode) bool {
	return false
}

// Up dictates choice for promoting nodes after deletion  - NOOP
func (_ SingleOrder) Up() func(CNode) bool {
	return nil
}

// New constructs a new in-memory Node
func (_ SingleOrder) New() Node {
	return &MemNode{}
}

// Depth returns the length of a key
func (s SingleOrder) Depth() int {
	return s.depth
}

// Close NOOP
func (_ SingleOrder) Close() error {
	return nil
}

// Save NOOP
func (_ SingleOrder) Save(_ []byte) error {
	return nil
}

// Load NOOP
func (so SingleOrder) Load() (Node, bool, error) {
	return so.New(), false, nil
}

// Update is mode specific pot update function - NOOP just proxies to pkg wide default
func (so SingleOrder) Update(root Node, k []byte, f func(Entry) Entry) (Node, error) {
	update := Update(so.New(), NewAt(0, root), k, f, so)
	return update, nil
}

// Mode for persisted pots
type PersistedPot struct {
	Mode                     // non-persisted mode
	ls   persister.LoadSaver // persister interface to save pointer based data structure nodes
	dir  string              // the directory containing the root node reference file
	newf func() Entry        // pot entry contructor function
}

// NewPersistedPot constructs a Mode for persisted pots
func NewPersistedPot(dir string, mode Mode, ls persister.LoadSaver, newf func() Entry) *PersistedPot {
	return &PersistedPot{Mode: mode, dir: dir, ls: ls, newf: newf}
}

// newPacked constructs a packed node that allows loading via its reference
func (pm *PersistedPot) NewPacked(ref []byte) *DBNode {
	return &DBNode{newf: pm.newf, ref: ref}
}

// Load loads the pot by reading the root reference from a file and creating the root node
func (pm *PersistedPot) Load() (r Node, loaded bool, err error) {
	rootfile := path.Join(pm.dir, "root")
	if _, err := os.Stat(rootfile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	ref, err := os.ReadFile(rootfile)
	if err != nil {
		return nil, false, err
	}
	root := pm.NewPacked(ref)
	root.MemNode = &MemNode{}
	if err := persister.Load(context.Background(), pm.ls, root); err != nil {
		return root, true, fmt.Errorf("failed to load persisted pot root node at '%x': %v", ref, err)
	}
	return root, true, nil
}

// Save persists the root node reference
func (pm *PersistedPot) Save(rootRef []byte) error {
	rootfile := path.Join(pm.dir, "root")
	return os.WriteFile(rootfile, rootRef, 0644)
}

// Close closes the loadsaver
func (pm *PersistedPot) Close() error {
	return pm.ls.Close()
}

// Update builds on the generic Update
func (pm *PersistedPot) Update(root Node, k []byte, f func(Entry) Entry) (Node, error) {
	update := Update(pm.New(), NewAt(0, root), k, f, pm)
	if update == nil {
		return update, nil
	}
	if err := pm.Save(update.(persister.TreeNode).Reference()); err != nil {
		return nil, err
	}
	return update, nil
}

// Pack serialises and saves the object
// once a new node is saved it can be delinked as node from memory
func (pm *PersistedPot) Pack(n Node) error {
	if n == nil {
		return nil
	}
	return persister.Save(context.Background(), pm.ls, n.(*DBNode))
}

// Unpack loads and deserialises node into memory
func (pm *PersistedPot) Unpack(n Node) error {
	if n == nil {
		return nil
	}
	dn := n.(*DBNode)
	if dn.MemNode != nil {
		return nil
	}
	dn.MemNode = &MemNode{}
	return persister.Load(context.Background(), pm.ls, dn)
}

// New constructs a new node
func (pm *PersistedPot) New() Node {
	return &DBNode{newf: pm.newf, MemNode: &MemNode{}}
}

package indigo

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/ethersphere/bee/pkg/indigo/persister"
	"github.com/ethersphere/bee/pkg/indigo/pot"
)

type Index struct {
	ls     persister.LoadSaver
	fh     io.ReadWriteCloser
	mode   *pot.PersistedPot
	read   chan pot.Node
	write  chan pot.Node
	update chan pot.Node
	quit   chan struct{}
}

func New(dir, name string, entryf func() pot.Entry, mode pot.Mode) (*Index, error) {
	ls, err := NewLoadSaver(dir)
	if err != nil {
		return nil, err
	}

	index := &Index{
		ls:     ls,
		mode:   pot.NewPersistedPot(mode, entryf),
		read:   make(chan pot.Node),
		write:  make(chan pot.Node),
		update: make(chan pot.Node),
		quit:   make(chan struct{}),
	}
	index.fh, err = os.OpenFile(path.Join(dir, fmt.Sprintf("index_%s", name)), os.O_RDWR|os.O_CREATE, 0644)
	ref, err := ioutil.ReadAll(index.fh)
	if err != nil {
		return nil, err
	}
	root := index.mode.NewFromReference(ref)
	if len(ref) > 0 {
		if err := persister.Load(context.Background(), index.ls, root); err != nil {
			return nil, err
		}
	}
	go index.start(root)
	return index, nil
}

// this forever loop is a locking mechanism for the pot index
// it allows only a single write operation at a time but multiple reads
//
func (idx *Index) start(root pot.Node) {
	write := idx.write
	quit := idx.quit
	for {
		select {
		case <-quit:
			if write == nil { // if in the middle of a write then wait till update
				quit = nil
			} else {
				return
			}
		case update := <-idx.update:
			if update != nil {
				root = update
			}
			if quit == nil { // if quitting, then quit
				return
			}
			write = idx.write
		case idx.read <- root:
		case write <- root: // write locks the pot until they recept or despara
			write = nil
		}
	}
}

func (idx *Index) Add(ctx context.Context, e pot.Entry) {
	root := <-idx.write
	root = pot.Add(root, e, &pot.SingleOrder{})
	idx.update <- root
}

func (idx *Index) Delete(ctx context.Context, k []byte) {
	root := <-idx.write
	root = pot.Delete(root, k, &pot.SingleOrder{})
	idx.update <- root
}

func (idx *Index) Find(ctx context.Context, k []byte) (pot.Entry, error) {
	return pot.Find(<-idx.read, k)
}

func (idx *Index) Close() error {
	close(idx.quit)
	return idx.ls.Close()
}
func (idx *Index) Iter(f func(pot.Entry)) {
	pot.Iter(pot.CNode{At: 0, Node: <-idx.read}, f)
}

func (idx *Index) Root() pot.Node {
	return <-idx.read
}

func (idx *Index) String() string {
	return idx.Root().String()
}

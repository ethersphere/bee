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
	fh     io.ReadWriteCloser
	entryf func() pot.Entry
	ls     persister.LoadSaver
	read   chan pot.Node
	write  chan pot.Node
	update chan pot.Node
	quit   chan struct{}
}

func New(dir, name string, entryf func() pot.Entry) (*Index, error) {
	ls, err := NewLoadSaver(dir)
	if err != nil {
		return nil, err
	}

	index := &Index{
		entryf: entryf,
		ls:     ls,
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
	root := pot.NewDBNode(ls, index.entryf)
	root.SetReference(ref)
	go index.start(root)
	return index, nil
}

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
		case write <- root:
			write = nil
		}
	}
}

func (idx *Index) Add(ctx context.Context, e pot.Entry) {
	root := <-idx.write
	root = pot.Update(root.New(), pot.NewCNode(root, 0), e.Key(), func(_ pot.Entry) pot.Entry { return e })
	idx.update <- root
}

func (idx *Index) Delete(ctx context.Context, k []byte) {
	root := <-idx.write
	root = pot.Update(root.New(), pot.NewCNode(root, 0), k, func(_ pot.Entry) pot.Entry { return nil })
	idx.update <- root
}

func (idx *Index) Find(ctx context.Context, k []byte) (pot.Entry, error) {
	return pot.Find(<-idx.read, k)
}

func (idx *Index) Close() error {
	close(idx.quit)
	return idx.ls.Close()
}

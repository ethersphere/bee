package indigo

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/indigo/pot"
)

type Index struct {
	mu   sync.Mutex
	root pot.Node
}

func New(dir string) (*Index, error) {
	ls, err := NewLoadSaver(dir)
	if err != nil {
		return nil, err
	}
	root := pot.NewDBNode(nil, ls)
	return &Index{root: root}, nil
}

func (idx *Index) Add(ctx context.Context, e pot.Entry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.root = pot.Update(idx.root.New(), pot.NewCNode(idx.root, 0), e.Key(), func(_ pot.Entry) pot.Entry { return e })
}

func (idx *Index) Delete(ctx context.Context, k []byte) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.root = pot.Update(idx.root.New(), pot.NewCNode(idx.root, 0), k, func(_ pot.Entry) pot.Entry { return nil })
}

func (idx *Index) Find(ctx context.Context, k []byte) (pot.Entry, error) {
	return pot.Find(idx.root.New(), k)
}

func (idx *Index) Close() error {
	return idx.root.Close()
}

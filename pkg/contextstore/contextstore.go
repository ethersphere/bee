package contextstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/contextstore/disc"
	"github.com/ethersphere/bee/pkg/contextstore/upload"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var errUnsupported = errors.New("unsupported operation")

type ContextStore struct {
	uploadContext *upload.Context
	pinContext    storage.NestedChunkStorer
	discContext   *disc.Context
	syncContext   storage.Storer
}

func New(path string, ls storage.Storer, ret retrieval.Interface) (*ContextStore, []chan *pusher.Op) {
	if err := os.Mkdir(filepath.Join(path, "upload"), 0755); err != nil {
		panic(err)
	}
	up := upload.New(filepath.Join(path, "upload"))
	discContext, pushC := disc.New(ret)
	c := &ContextStore{
		uploadContext: up,
		syncContext:   ls,
		discContext:   discContext,
	}
	return c, []chan *pusher.Op{up.FeedPusher(), pushC}
}

// ContextByName returns a writable context with the supplied name.
// When name is an empty string, a new context will be created.
// Returns the name of the context (same as request in the case of an existing one), the underlying store and a cleanup function.
func (c *ContextStore) ContextByName(sc storage.Context, name string) (string, storage.SimpleChunkStorer, func(), error) {
	switch sc {
	case storage.ContextUpload:
		if name == "" {
			return c.uploadContext.New()
		}
		st, cleanup, err := c.uploadContext.GetByName(name)
		return name, st, cleanup, err
	case storage.ContextPin:
		//... the same as
	}
	return "", nil, func() {}, errUnsupported
}

func (c *ContextStore) LookupContext() storage.SimpleChunkGetter {
	return &syntheticStore{
		seq: []storage.SimpleChunkGetter{
			c.uploadContext,
			wrap(c.syncContext, storage.ModeGetSync, storage.ModePutSync),
			c.discContext,
		},
	}
}

func (c *ContextStore) DiscContext() storage.SimpleChunkPutGetter {
	return c.discContext
}

func (c *ContextStore) PinContext() storage.NestedChunkStorer { return c.pinContext }

type syntheticStore struct {
	seq []storage.SimpleChunkGetter
}

func (s *syntheticStore) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	for _, st := range s.seq {
		if ch, err := st.Get(ctx, addr); err != nil {
			return ch, err
		}
	}
	return nil, storage.ErrNotFound
}

type modeWrapper struct {
	modeGet storage.ModeGet
	modePut storage.ModePut
	ls      storage.Storer
}

func wrap(l storage.Storer, g storage.ModeGet, p storage.ModePut) storage.SimpleChunkStorer {
	return &modeWrapper{
		modeGet: g,
		modePut: p,
		ls:      l,
	}
}

// Get a chunk by its swarm.Address. Returns the chunk associated with
// the address alongside with its postage stamp, or a storage.ErrNotFound
// if the chunk is not found.
func (m *modeWrapper) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return m.ls.Get(ctx, m.modeGet, addr)
}

// Put a chunk into the store alongside with its postage stamp. No duplicates
// are allowed. It returns `exists=true` In case the chunk already exists.
func (m *modeWrapper) Put(ctx context.Context, ch swarm.Chunk) (exists bool, err error) {
	e, err := m.ls.Put(ctx, m.modePut, ch)
	return e[0], err
}

// Iterate over chunks in no particular order.
func (m *modeWrapper) Iterate(_ storage.IterateChunkFn) error {
	panic("not implemented") // TODO: Implement
}

// Delete a chunk from the store.
func (m *modeWrapper) Delete(ctx context.Context, addr swarm.Address) error {
	return m.ls.Set(ctx, storage.ModeSetRemove, addr)
}

// Has checks whether a chunk exists in the store.
func (m *modeWrapper) Has(ctx context.Context, addr swarm.Address) (bool, error) {
	return m.ls.Has(ctx, addr)
}

func (m *modeWrapper) Count() (int, error) {
	panic("not implemented")
}

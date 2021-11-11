package disc

import (
	"context"

	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_ storage.SimpleChunkGetter = (*Context)(nil)
	_ storage.SimpleChunkPutter = (*Context)(nil)
)

type Context struct {
	chC chan *pusher.Op
	r   retrieval.Interface
}

func New(ret retrieval.Interface) (*Context, chan *pusher.Op) {
	c := &Context{
		chC: make(chan *pusher.Op),
		r:   ret,
	}

	return c, c.chC
}

// Put a chunk into the store alongside with its postage stamp. No duplicates
// are allowed.
func (c *Context) Put(ctx context.Context, ch swarm.Chunk) (exists bool, err error) {
	op := &pusher.Op{Chunk: ch, Err: make(chan error), Direct: true}

	// TODO needs context
	c.chC <- op

	return false, <-op.Err
}

// Get a chunk by its swarm.Address. Returns the chunk associated with
// the address alongside with its postage stamp, or a storage.ErrNotFound
// if the chunk is not found.
func (c *Context) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return c.r.RetrieveChunk(ctx, addr, true)
}

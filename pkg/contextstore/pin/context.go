package pin

import (
	"context"

	"github.com/ethersphere/bee/pkg/contextstore/nested"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ storage.PinStore = (*Context)(nil)

type Context struct {
	stores *nested.Store
	quit   chan struct{}
}

func New(base string) *Context {
	// still needs to be worked out:
	// how to make sure a collection that was uploaded and
	// pinned gets uploaded once, preserving the current behavior
	// of the api. this might require storing metadata over each collection
	// and for example toggling some flag on and off.
	// although, this can also be solved by requiring user interaction. for example,
	// we can change the behavior in a way that a pinned collection simply does not get
	// automatically upload. this way a user can keep on adding data to a pinned collection
	// and invoke upload by an iterator that would just walk through the collection and just upload it once.
	// this will be required anyway, either by doing it inside bee or by some higher level solution
	// that would execute this. simply put, the operation can be done by opening a sequential iterator on
	// the database that runs through the chunks and uploads them. similarly,the traversal service for pinned
	// content becomes a sequential iterator, not needing any real traversal of content and its underlying data
	// types (for pinning in hindsight this is not the case)
	u := &Context{
		stores: nested.Open(base),
	}

	return u
}

func (c *Context) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return c.stores.Get(ctx, addr)
}

func (c *Context) GetByName(name string) (storage.SimpleChunkStorer, func(), error) {
	return c.stores.GetByName(name)
}

func (c *Context) Delete(uuid string) error {
	return c.stores.Delete(uuid)
}

func (c *Context) EachStore(cb storage.EachStoreFunc) error {
	return c.stores.EachStore(cb)
}

func (c *Context) New() (string, storage.SimpleChunkStorer, func(), error) {
	return c.stores.New()
}

func (c *Context) Pins() ([]swarm.Address, error) {
	var roots []swarm.Address
	err := c.stores.EachStore(func(root string, _ storage.SimpleChunkStorer) (stop bool, err error) {
		roots = append(roots, swarm.MustParseHexAddress(root))
		return false, nil
	})

	return roots, err
}

func (c *Context) Unpin(root swarm.Address) error {
	return c.Delete(root.String())
}

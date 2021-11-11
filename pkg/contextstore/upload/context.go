package upload

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/pkg/contextstore/nested"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Context struct {
	stores *nested.Store
	wg     sync.WaitGroup
	quit   chan struct{}
}

func New(base string) *Context {
	u := &Context{
		stores: nested.Open(base),
	}

	return u
}

// FeedPusher feeds the pusher continuously with chunks for the different
// uploads and makes sure that empty stores get cleaned up, reclaiming disk space.
func (c *Context) FeedPusher() chan *pusher.Op {
	c.wg.Add(1)
	out := make(chan *pusher.Op)

	go func() {
		defer c.wg.Done()
		defer close(out)
		for {
			// pump the data from each store
			// if the chunk synced succesfully - delete it
			// one the store reaches count 0 - destroy it
			var addresses []swarm.Address
			var empty []string
			err := c.stores.EachStore(func(uuid string, st storage.SimpleChunkStorer) (bool, error) {
				if err := st.Iterate(func(ch swarm.Chunk) (bool, error) {

					// push the chunk to sync. if everything is ok, collect the address
					// then delete the chunk from the db. if not, continue as is... (mimics the old behavior)
					op := &pusher.Op{Chunk: ch, Err: make(chan error), Direct: true}
					select {
					case out <- op:
						//todo: case quit
					}

					select {
					case err := <-op.Err:
						if err == nil {
							// chunk pushed succesfully, collect for cleanup
							// potentially keep some metadata in the store about already synced chunks
							// and just skip it next time.
							addresses = append(addresses, ch.Address())
						}
					case <-c.quit:
						return true, nil
					}

					return false, nil
				}); err != nil {
					// handle error
				}
				ctx := context.Background()
				for _, addr := range addresses {
					if st.Delete(ctx, addr) != nil {
						// log and continue?
					}
				}

				addresses = nil

				if cnt, err := st.Count(); err == nil && cnt == 0 {
					// store syncing is done, mark it for deletion
					// however note that deletion can only take place after the EachStore
					// callback returns (the following return line) due to the protection
					// with a waitgroup over the nested store entry.
					empty = append(empty, uuid)
				}
				return false, nil
			})
			if !errors.Is(err, nested.ErrStoreChange) {
				//log the error and back off. need to enumerate what can happen here to handle better
			}

			// cleanups
			for _, uuid := range empty {
				c.stores.Delete(uuid)
			}

			select {
			case <-c.quit:
				return
			}
		}
	}()
	return out
}

// Left indicates how many chunks left in an upload store (can be used to track progress).
func (c *Context) Left(uuid string) (left, total int, err error) {
	st, cleanup, err := c.stores.GetByName(uuid)
	if err != nil {
		return 0, 0, err
	}
	defer cleanup()

	left, err = st.Count()

	// for total -> need to count total writes into the db and this can be erroneous
	return left, 0, err
}

func (c *Context) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return c.stores.Get(ctx, addr)
}

func (c *Context) GetByName(name string) (storage.SimpleChunkStorer, func(), error) {
	return c.stores.GetByName(name)
}

func (c *Context) New() (string, storage.SimpleChunkStorer, func(), error) {
	return c.stores.New()
}

func (c *Context) Close() error {
	close(c.quit)
	c.wg.Wait()
	return c.stores.Close()
}

package pebblestore

import (
	"context"
	"fmt"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

// Batch implements storage.BatchedStore interface Batch method.
func (s *Store) Batch(ctx context.Context) storage.Batch {
	return &Batch{
		ctx:   ctx,
		batch: new(pebble.Batch),
		store: s,
	}
}

type Batch struct {
	ctx context.Context

	mu    sync.Mutex // mu guards batch and done.
	batch *pebble.Batch
	store *Store
	done  bool
}

// Put implements storage.Batch interface Put method.
func (b *Batch) Put(item storage.Item) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}

	val, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("unable to marshal item: %w", err)
	}

	b.mu.Lock()
	b.batch.Set(key(item), val, pebble.Sync)
	b.mu.Unlock()

	return nil
}

// Delete implements storage.Batch interface Delete method.
func (b *Batch) Delete(item storage.Item) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}

	b.mu.Lock()
	b.batch.Delete(key(item), pebble.Sync)
	b.mu.Unlock()

	return nil
}

// Commit implements storage.Batch interface Commit method.
func (b *Batch) Commit() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.done {
		return storage.ErrBatchCommitted
	}

	if err := b.batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("unable to commit batch: %w", err)
	}

	b.done = true

	return nil
}

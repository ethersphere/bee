package rocksdb

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/linxGnu/grocksdb"
)

// Batch implements storage.BatchedStore interface Batch method.
func (s *Store) Batch(ctx context.Context) storage.Batch {
	return &Batch{
		ctx:   ctx,
		batch: grocksdb.NewWriteBatch(),
		store: s,
	}
}

type Batch struct {
	ctx context.Context

	mu    sync.Mutex // mu guards batch and done.
	batch *grocksdb.WriteBatch
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
	b.batch.Put(key(item), val)
	b.mu.Unlock()

	return nil
}

// Delete implements storage.Batch interface Delete method.
func (b *Batch) Delete(item storage.Item) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}

	b.mu.Lock()
	b.batch.Delete(key(item))
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

	if err := b.store.db.Write(grocksdb.NewDefaultWriteOptions(), b.batch); err != nil {
		return fmt.Errorf("unable to commit batch: %w", err)
	}

	b.done = true

	return nil
}

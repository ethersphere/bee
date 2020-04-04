package shed

import (
	"context"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/disk"
	"github.com/ethersphere/bee/pkg/storage/mem"
)

// DB provides abstractions over LevelDB in order to
// implement complex structures using fields and ordered indexes.
// It provides a schema functionality to store fields and indexes
// information about naming and types.
type DB struct {
	Store storage.Storer
}

func NewDB(path string) (db *DB, err error) {
	if path == "" {
		ms, err := mem.NewMemStorer(storage.ValidateContentChunk)
		if err != nil {
			return nil, err
		}
		return &DB{
			Store: ms,
		}, nil
	} else {
		ds, err := disk.NewDiskStorer(path, storage.ValidateContentChunk)
		if err != nil {
			return nil, err
		}

		db := &DB{
			Store: ds,
		}

		ctx := context.Background()
		if _, err = db.getSchema(ctx); err != nil {
			if err == storage.ErrNotFound {
				// save schema with initialized default fields
				if err = db.putSchema(ctx, schema{
					Fields:  make(map[string]fieldSpec),
					Indexes: make(map[byte]indexSpec),
				}); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}

		return db, nil
	}
}


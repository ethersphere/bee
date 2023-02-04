package localstore

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/pusher"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

func (db *DB) DirectUpload() storage.Putter {
	return storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
		op := &pusher.Op{Chunk: ch, Err: make(chan error), Direct: true}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-db.quit:
			return errors.New("db stopped")
		case db.pusherFeed <- op:
			return <-op.Err
		}
	})
}

func (db *DB) Download() storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
		ch, err := db.Lookup().Get(ctx, address)
		switch {
		case err == nil:
			return ch, nil
		case errors.Is(err, storage.ErrNotFound):
			// if chunk is not found locally, retrieve it from the network
			ch, err = db.retrieval.RetrieveChunk(ctx, address, swarm.ZeroAddress)
			if err == nil {
				_ = db.Cache().Put(ctx, ch)
			}
		}
		if err != nil {
			return nil, err
		}
		return ch, nil
	})
}

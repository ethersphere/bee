package transaction

import (
	"context"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
)

type Txn interface {
	Ctx() context.Context
	Store() storage.Store
	ChunkStore() storage.ChunkStore

	Commit() error
}

type txn struct {
	ctx    context.Context
	st     storage.Store
	batch  storage.Batch
	sharky *sharky.Store
}

func New(
	ctx context.Context,
	st storage.BatchedStore,
	sharky *sharky.Store,
) (Txn, error) {

	batch, err := st.Batch(ctx)
	if err != nil {
		return nil, err
	}

	return &txn{
		ctx:    ctx,
		st:     st,
		batch:  batch,
		sharky: sharky,
	}, nil
}

func (t *txn) Ctx() context.Context {
	return t.ctx
}

func (t *txn) Store() storage.Store {
	return nil
}

func (t *txn) ChunkStore() storage.ChunkStore {
	return chunkstore.New(t.Store(), t.sharky)
}

func (t *txn) Commit() error {
	return nil
}

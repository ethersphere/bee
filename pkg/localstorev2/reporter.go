package storer

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Report implements the storage.PushReporter by wrapping the internal reporter
// with a transaction.
func (db *DB) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {
	txnRepo, commit, rollback := db.repo.NewTx(ctx)
	reporter := upload.NewPushReporter(txnRepo)

	err := reporter.Report(ctx, chunk, state)
	if err != nil {
		return errors.Join(err, rollback())
	}

	return commit()
}

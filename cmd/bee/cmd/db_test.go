package cmd_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/cmd/bee/cmd"
	"github.com/ethersphere/bee/pkg/postage"
	testing2 "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer"
	kademlia "github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

func TestDBExportImport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t, ctx, &storer.Options{
		Batchstore:      new(postage.NoOpBatchStore),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Logger:          testutil.NewLogger(t),
		ReserveCapacity: 4_194_304,
	})

	nChunks := 10
	for i := 0; i < nChunks; i++ {
		ch := testing2.GenerateTestRandomChunk()
		err := db.ReservePut(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("put chunk: ", ch.Address().String())
	}

	err := newCommand(t, cmd.WithArgs("db", "export", "reserve", "export.tar", "--data-dir", "./test/")).Execute()
	if err != nil {
		t.Fatal(err)
	}

	err = newCommand(t, cmd.WithArgs("db", "import", "reserve", "export.tar", "--data-dir", "./test/")).Execute()
	if err != nil {
		t.Fatal(err)
	}
}

func newTestDB(t *testing.T, ctx context.Context, opts *storer.Options) *storer.DB {
	t.Helper()
	db, err := storer.New(ctx, "test", opts)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, db)
	return db
}

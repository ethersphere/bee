package cmd_test

import (
	"context"
	"fmt"
	"github.com/ethersphere/bee/cmd/bee/cmd"
	testing2 "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/util/testutil"
	"testing"
)

func TestDBExportImport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t, ctx, &storer.Options{
		Batchstore:   nil,
		RadiusSetter: nil,
		Logger:       testutil.NewLogger(t),
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
	//testutil.CleanupCloser(t, db)
	return db
}

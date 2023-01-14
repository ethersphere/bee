package chunkstore_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/sharky"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/afero"
)

func TestTxChunkStore(t *testing.T) {
	t.Parallel()

	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	storagetest.TestTxChunkStore(t, chunkstore.NewTxChunkStore(inmemstore.NewTxStore(inmemstore.New()), sharky))
}

func TestMultipleStamps(t *testing.T) {
	t.Parallel()

	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	chunkStore := chunkstore.NewTxChunkStore(inmemstore.NewTxStore(inmemstore.New()), sharky)

	chunk := chunktest.GenerateTestRandomChunk()
	stamps := []swarm.Stamp{chunk.Stamp()}
	for i := 0; i < 2; i++ {
		stamps = append(stamps, postagetesting.MustNewStamp())
	}

	verify := func(t *testing.T) {
		t.Helper()

		rIdx := chunkstore.RetrievalIndexItem{
			Address: chunk.Address(),
		}

		has, err := chunkStore.(*chunkstore.TxChunkStoreWrapper).Store().Has(&rIdx)
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatalf("retrievalIndex not found %s", chunk.Address())
		}

		for _, stamp := range stamps {
			sIdx := chunkstore.ChunkStampItem{
				Address: chunk.Address(),
				Stamp:   stamp,
			}

			has, err := chunkStore.(*chunkstore.TxChunkStoreWrapper).Store().Has(&sIdx)
			if err != nil {
				t.Fatal(err)
			}

			if !has {
				t.Fatalf("chunkStampItem not found %s", chunk.Address())
			}
		}
	}

	t.Run("put with multiple stamps", func(t *testing.T) {
		cs := chunkStore.NewTx(storage.NewTxState(context.TODO()))

		for _, stamp := range stamps {
			err := chunkStore.Put(context.TODO(), chunk.WithStamp(stamp))
			if err != nil {
				t.Fatalf("failed to put chunk: %v", err)
			}
		}

		err := cs.Commit()
		if err != nil {
			t.Fatal(err)
		}

		verify(t)
	})

	t.Run("rollback delete operations", func(t *testing.T) {
		t.Run("less than refCnt", func(t *testing.T) {
			cs := chunkStore.NewTx(storage.NewTxState(context.TODO()))

			for i := 0; i < 2; i++ {
				err := cs.Delete(context.TODO(), chunk.Address())
				if err != nil {
					t.Fatal(err)
				}
			}

			err := cs.Rollback()
			if err != nil {
				t.Fatal(err)
			}

			verify(t)
		})

		// this should remove all the stamps and hopefully bring them back
		t.Run("till refCnt", func(t *testing.T) {
			cs := chunkStore.NewTx(storage.NewTxState(context.TODO()))

			for i := 0; i < 3; i++ {
				err := cs.Delete(context.TODO(), chunk.Address())
				if err != nil {
					t.Fatal(err)
				}
			}

			err := cs.Rollback()
			if err != nil {
				t.Fatal(err)
			}

			verify(t)
		})
	})
}

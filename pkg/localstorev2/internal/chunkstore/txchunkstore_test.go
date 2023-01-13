package chunkstore_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/afero"
)

func TestTxChunkStore(t *testing.T) {
	t.Parallel()

	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.ChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	storagetest.TestTxChunkStore(t, chunkstore.NewTxChunkStore(inmemstore.NewTxStore(inmemstore.New()), sharky))
}

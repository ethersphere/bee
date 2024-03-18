// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
}

func Test_Step_04(t *testing.T) {
	t.Parallel()

	sharkyDir := t.TempDir()
	sharkyStore, err := sharky.New(&dirFS{basedir: sharkyDir}, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	store := inmemstore.New()
	chStore := chunkstore.New(store, sharkyStore)
	stepFn := localmigration.Step_04(sharkyDir, 1)

	chunks := chunktest.GenerateTestRandomChunks(10)

	for _, ch := range chunks {
		err := chStore.Put(context.Background(), ch)
		assert.NoError(t, err)
	}

	for _, ch := range chunks[:2] {
		err := store.Delete(&chunkstore.RetrievalIndexItem{Address: ch.Address()})
		assert.NoError(t, err)
	}

	err = sharkyStore.Close()
	assert.NoError(t, err)

	assert.NoError(t, stepFn(store))

	sharkyStore, err = sharky.New(&dirFS{basedir: sharkyDir}, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	chStore = chunkstore.New(store, sharkyStore)

	// check that the chunks are still there
	for _, ch := range chunks[2:] {
		_, err := chStore.Get(context.Background(), ch.Address())
		assert.NoError(t, err)
	}

	err = sharkyStore.Close()
	assert.NoError(t, err)

	// check that the sharky files are there
	f, err := os.Open(filepath.Join(sharkyDir, "free_000"))
	assert.NoError(t, err)

	buf := make([]byte, 2)
	_, err = f.Read(buf)
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		if i < 2 {
			// if the chunk is deleted, the bit is set to 1
			assert.Greater(t, buf[i/8]&(1<<(i%8)), byte(0))
		} else {
			// if the chunk is not deleted, the bit is 0
			assert.Equal(t, byte(0), buf[i/8]&(1<<(i%8)))
		}
	}

	assert.NoError(t, f.Close())
}

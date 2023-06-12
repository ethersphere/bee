// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/log"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/shed"
	mockstatestore "github.com/ethersphere/bee/pkg/statestore/mock"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/swarm"
)

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
}

func createOldDataDir(t *testing.T, dataPath string, baseAddress swarm.Address, stateStore storage.StateStorer) {
	t.Helper()

	binIDs := map[uint8]int{}

	assignBinID := func(addr swarm.Address) int {
		po := swarm.Proximity(baseAddress.Bytes(), addr.Bytes())
		if _, ok := binIDs[po]; !ok {
			binIDs[po] = 1
			return 1
		}
		binIDs[po]++
		return binIDs[po]
	}

	err := os.Mkdir(filepath.Join(dataPath, "sharky"), 0777)
	if err != nil {
		t.Fatal(err)
	}

	sharkyStore, err := sharky.New(&dirFS{basedir: filepath.Join(dataPath, "sharky")}, 2, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	defer sharkyStore.Close()

	shedDB, err := shed.NewDB(dataPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer shedDB.Close()

	pIdx, rIdx, err := storer.InitShedIndexes(shedDB, baseAddress)
	if err != nil {
		t.Fatal(err)
	}

	reserveChunks := chunktest.GenerateTestRandomChunks(10)

	for _, c := range reserveChunks {
		loc, err := sharkyStore.Write(context.Background(), c.Data())
		if err != nil {
			t.Fatal(err)
		}

		locBuf, err := loc.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}

		binID := assignBinID(c.Address())

		err = pIdx.Put(shed.Item{
			Address: c.Address().Bytes(),
			BinID:   uint64(binID),
			BatchID: c.Stamp().BatchID(),
		})
		if err != nil {
			t.Fatal(err)
		}

		err = rIdx.Put(shed.Item{
			Address:   c.Address().Bytes(),
			BinID:     uint64(binID),
			BatchID:   c.Stamp().BatchID(),
			Index:     c.Stamp().Index(),
			Timestamp: c.Stamp().Timestamp(),
			Sig:       c.Stamp().Sig(),
			Location:  locBuf,
		})

		if err != nil {
			t.Fatal(err)
		}
	}

	// create a pinning collection
	writer := splitter.NewSimpleSplitter(
		storage.PutterFunc(
			func(ctx context.Context, chunk swarm.Chunk) error {
				c := chunk.WithStamp(postagetesting.MustNewStamp())

				loc, err := sharkyStore.Write(context.Background(), c.Data())
				if err != nil {
					return err
				}

				locBuf, err := loc.MarshalBinary()
				if err != nil {
					return err
				}

				return rIdx.Put(shed.Item{
					Address:   c.Address().Bytes(),
					BatchID:   c.Stamp().BatchID(),
					Index:     c.Stamp().Index(),
					Timestamp: c.Stamp().Timestamp(),
					Sig:       c.Stamp().Sig(),
					Location:  locBuf,
				})
			},
		),
	)

	randData := make([]byte, 4096*20)
	_, err = rand.Read(randData)
	if err != nil {
		t.Fatal(err)
	}

	root, err := writer.Split(context.Background(), io.NopCloser(bytes.NewBuffer(randData)), 4096*20, false)
	if err != nil {
		t.Fatal(err)
	}

	err = stateStore.Put(fmt.Sprintf("root-pin-%s", root.String()), root)
	if err != nil {
		t.Fatal(err)
	}
}

type testSharkyRecovery struct {
	*sharky.Recovery
	mtx      sync.Mutex
	addCalls int
}

func (t *testSharkyRecovery) Add(loc sharky.Location) error {
	t.mtx.Lock()
	t.addCalls++
	t.mtx.Unlock()
	return t.Recovery.Add(loc)
}

type testReservePutter struct {
	mtx   sync.Mutex
	size  int
	calls int
}

func (t *testReservePutter) Put(ctx context.Context, st internal.Storage, ch swarm.Chunk) (bool, error) {
	t.mtx.Lock()
	t.calls++
	t.mtx.Unlock()
	return true, st.ChunkStore().Put(ctx, ch)
}

func (t *testReservePutter) AddSize(size int) {
	t.mtx.Lock()
	t.size += size
	t.mtx.Unlock()
}

func (t *testReservePutter) Size() int {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.size
}

func TestEpochMigration(t *testing.T) {
	t.Parallel()

	var (
		dataPath    = t.TempDir()
		baseAddress = swarm.RandAddress(t)
		stateStore  = mockstatestore.NewStateStore()
		reserve     = &testReservePutter{}
		logBytes    = bytes.NewBuffer(nil)
		logger      = log.NewLogger("test", log.WithSink(logBytes))
		indexStore  = inmemstore.New()
	)

	createOldDataDir(t, dataPath, baseAddress, stateStore)

	r, err := sharky.NewRecovery(path.Join(dataPath, "sharky"), 2, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	sharkyRecovery := &testSharkyRecovery{Recovery: r}

	err = storer.EpochMigration(
		context.Background(),
		dataPath,
		stateStore,
		indexStore,
		reserve,
		sharkyRecovery,
		logger,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.ContainsAny(logBytes.String(), "migrating pinning collections done") {
		t.Fatalf("expected log to contain 'migrating pinning collections done', got %s", logBytes.String())
	}

	if !strings.ContainsAny(logBytes.String(), "migrating reserve contents done") {
		t.Fatalf("expected log to contain 'migrating pinning collections done', got %s", logBytes.String())
	}

	if sharkyRecovery.addCalls != 31 {
		t.Fatalf("expected 31 add calls, got %d", sharkyRecovery.addCalls)
	}

	if reserve.calls != 10 {
		t.Fatalf("expected 10 reserve calls, got %d", reserve.calls)
	}

	if reserve.size != 10 {
		t.Fatalf("expected 10 reserve size, got %d", reserve.size)
	}

	pins, err := pinstore.Pins(indexStore)
	if err != nil {
		t.Fatal(err)
	}

	if len(pins) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(pins))
	}

	if !strings.ContainsAny(logBytes.String(), pins[0].String()) {
		t.Fatalf("expected log to contain root pin reference, got %s", logBytes.String())
	}
}

package storer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	sharkyDirtyFileName = ".DIRTY"
)

func sharkyRecovery(sharkyBasePath string, store storage.Store, opts *Options) (closerFn, error) {
	logger := opts.Logger.WithName(loggerName).Register()
	dirtyFilePath := filepath.Join(sharkyBasePath, sharkyDirtyFileName)

	closer := func() error { return os.Remove(dirtyFilePath) }

	if _, err := os.Stat(dirtyFilePath); errors.Is(err, fs.ErrNotExist) {
		return closer, os.WriteFile(dirtyFilePath, []byte{}, 0644)
	}

	logger.Info("localstore sharky .DIRTY file exists: starting recovery due to previous dirty exit")
	defer func(t time.Time) {
		logger.Info("localstore sharky recovery finished", "time", time.Since(t))
	}(time.Now())

	sharkyRecover, err := sharky.NewRecovery(sharkyBasePath, sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return closer, err
	}

	defer func() {
		if err := sharkyRecover.Close(); err != nil {
			logger.Error(err, "failed closing sharky recovery")
		}
	}()

	locationResultC := make(chan locationResult, 128)

	go readSharkyLocations(locationResultC, store)

	if err := addLocations(locationResultC, sharkyRecover); err != nil {
		return closer, err
	}

	return closer, nil
}

type locationResult struct {
	err error
	loc sharky.Location
}

func readSharkyLocations(locationResultC chan<- locationResult, store storage.Store) {
	defer close(locationResultC)

	err := store.Iterate(storage.Query{
		Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItemTemp) },
	}, func(r storage.Result) (bool, error) {
		entry := r.Entry.(*chunkstore.RetrievalIndexItemTemp)
		locationResultC <- locationResult{loc: entry.Location}
		return false, nil
	})
	if err != nil {
		locationResultC <- locationResult{err: fmt.Errorf("iterate index: %w", err)}
	}
}

func addLocations(locationResultC <-chan locationResult, sharkyRecover *sharky.Recovery) error {
	for res := range locationResultC {
		if res.err != nil {
			return res.err
		}

		if err := sharkyRecover.Add(res.loc); err != nil {
			return err
		}
	}

	if err := sharkyRecover.Save(); err != nil {
		return err
	}

	return nil
}

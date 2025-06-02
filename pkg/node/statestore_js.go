//go:build js

//+go:build js

package node

import (
	"path/filepath"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/statestore/storeadapter"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/cache"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
)

// InitStateStore will initialize the stateStore with the given path to the
// data directory. When given an empty directory path, the function will instead
// initialize an in-memory state store that will not be persisted.
func InitStateStore(logger log.Logger, dataDir string, cacheCapacity uint64) (storage.StateStorerManager, *cache.Cache, error) {
	if dataDir == "" {
		logger.Warning("using in-mem state store, no node state will be persisted")
	} else {
		dataDir = filepath.Join(dataDir, "statestore")
	}
	ldb, err := leveldbstore.New(dataDir, nil)
	if err != nil {
		return nil, nil, err
	}

	caching, err := cache.Wrap(ldb, int(cacheCapacity))
	if err != nil {
		return nil, nil, err
	}

	stateStore, err := storeadapter.NewStateStorerAdapter(caching)

	return stateStore, caching, err
}

package main

import (
	"os"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
)

func main() {
	logger := logging.New(os.Stdout, 5)
	logger.Info("debug indices initializing...")
	st, err := leveldb.NewInMemoryStateStore(logger)
	if err != nil {
		panic(err)
	}
	path := os.Args[1]
	//lo := &localstore.Options{
	//OpenFilesLimit:         256,
	//BlockCacheCapacity:     32 * 1024 * 1024,
	//WriteBufferSize:        32 * 1024 * 1024,
	//DisableSeeksCompaction: false,
	//}

	addr := []byte{31: 0}
	storer, err := localstore.New(path, addr, st, nil, logger)
	if err != nil {
		panic(err)
	}
	defer storer.Close()
	//ind, err := storer.DebugIndices()
	//if err != nil {
	//panic(err)
	//}

	//logger.Infof("indices:\n%s", ind)
}

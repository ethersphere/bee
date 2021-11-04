package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	inserts = 4000000
	reserve = 10000000
	cache   = 50000
)

func main() {
	logger := logging.New(os.Stdout, 5)
	st, err := leveldb.NewInMemoryStateStore(logger)
	if err != nil {
		panic(err)
	}
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	path = filepath.Join(path, "data")
	fmt.Printf("using datadir in: '%s'\n", path)
	var (
		storageRadius = uint8(0)
		batches       [][]byte
		batchesMap    = make(map[string]uint8)
		chmtx, mtx    sync.Mutex

		rdchs, chs []swarm.Address
		done       = make(chan struct{})
		wg         sync.WaitGroup
		pos        = 0
	)

	unreserve := func(cb postage.UnreserveIteratorFn) error {
		logger.Infof("reserve eviction triggered. current storage radius %d, offest %d", storageRadius, pos)
		start := time.Now()
		defer func() {
			logger.Infof("reserve eviction done, took %v", time.Since(start))
		}()

		mtx.Lock()
		defer mtx.Unlock()
		for {
			for k, v := range batchesMap {
				stop, err := cb([]byte(k), v)
				if err != nil {
					return err
				}
				batchesMap[k] = v + 1
				if stop {
					return nil
				}
			}
		}
	}

	lo := &localstore.Options{
		Capacity:               cache,
		ReserveCapacity:        reserve,
		UnreserveFunc:          unreserve,
		OpenFilesLimit:         256,
		BlockCacheCapacity:     32 * 1024 * 1024,
		WriteBufferSize:        32 * 1024 * 1024,
		DisableSeeksCompaction: false,
	}

	addr := []byte{31: 0}
	storer, err := localstore.New(path, addr, st, lo, logger)
	if err != nil {
		panic(err)
	}
	defer storer.Close()
	ctx := context.Background()
	wg.Add(1)
	// one goroutine inserts data
	go func() {
		defer wg.Done()
		f, _ := os.OpenFile("put_up_reserve_size.csv", os.O_RDWR|os.O_CREATE, 0666)
		f.Write([]byte("res,pututime\n"))
		f2, _ := os.OpenFile("put_up_time.csv", os.O_RDWR|os.O_CREATE, 0666)
		f2.Write([]byte("chunks,put100time\n"))
		start1000 := time.Now()
		startt := time.Now()
		for i := 0; i <= inserts; i++ {
			ch := testing.GenerateTestRandomChunk()
			start := time.Now()
			_, err := storer.Put(ctx, storage.ModePutUpload, ch)
			if err != nil {
				logger.Errorf("error putting uploaded chunk: %v", err)
			}
			end := int(time.Since(start).Microseconds())
			if i%100 == 0 {
				rsz := storer.ReserveSize()
				f.Write([]byte(fmt.Sprintf("%d,%d\n", rsz, end)))
			}
			chmtx.Lock()
			chs = append(chs, ch.Address())
			rdchs = append(chs, ch.Address())
			chmtx.Unlock()
			mtx.Lock()
			batches = append(batches, ch.Stamp().BatchID())
			batchesMap[string(ch.Stamp().BatchID())] = 0
			mtx.Unlock()
			if i%1000 == 0 {
				logger.Infof("wrote %d chunks", i)
				stop1000 := int(time.Since(start1000).Milliseconds())
				f2.Write([]byte(fmt.Sprintf("%d,%d\n", i, stop1000)))
				start1000 = time.Now()
			}
		}
		fmt.Println("finished inserting, took", time.Since(startt))
		time.Sleep(5 * time.Second)
		close(done)
	}()

	// wait for the data to be inserted, then try to iterate over the index
	wg.Wait()

	t2 := time.Now()
	ent := storer.RebuildFree()
	fmt.Printf("took %s to rebuild %d entries", time.Since(t2), ent)
}

// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Validate ensures that all retrievalIndex chunks are correctly stored in sharky.
func ValidateReserve(ctx context.Context, basePath string, opts *Options) error {

	logger := opts.Logger

	store, err := initStore(basePath, opts)
	if err != nil {
		return fmt.Errorf("failed creating levelDB index store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error(err, "failed closing store")
		}
	}()

	sharky, err := sharky.New(&dirFS{basedir: path.Join(basePath, sharkyPath)},
		sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return err
	}
	defer func() {
		if err := sharky.Close(); err != nil {
			logger.Error(err, "failed closing sharky")
		}
	}()

	logger.Info("performing chunk validation")

	validateWork(logger, store, sharky.Read)

	return nil
}

// ValidateRetrievalIndex ensures that all retrievalIndex chunks are correctly stored in sharky.
func ValidateRetrievalIndex(ctx context.Context, basePath string, opts *Options) error {

	logger := opts.Logger

	store, err := initStore(basePath, opts)
	if err != nil {
		return fmt.Errorf("failed creating levelDB index store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error(err, "failed closing store")
		}
	}()

	sharky, err := sharky.New(&dirFS{basedir: path.Join(basePath, sharkyPath)},
		sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return err
	}
	defer func() {
		if err := sharky.Close(); err != nil {
			logger.Error(err, "failed closing sharky")
		}
	}()

	logger.Info("performing chunk validation")
	validateWork(logger, store, sharky.Read)

	return nil
}

func validateWork(logger log.Logger, store storage.Store, readFn func(context.Context, sharky.Location, []byte) error) {

	total := 0
	socCount := 0
	invalidCount := 0

	n := time.Now()
	defer func() {
		logger.Info("validation finished", "duration", time.Since(n), "invalid", invalidCount, "soc", socCount, "total", total)
	}()

	iteratateItemsC := make(chan *chunkstore.RetrievalIndexItem)

	validChunk := func(item *chunkstore.RetrievalIndexItem, buf []byte) {
		err := readFn(context.Background(), item.Location, buf)
		if err != nil {
			logger.Warning("invalid chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0), "location", item.Location, "error", err)
			return
		}

		ch := swarm.NewChunk(item.Address, buf)
		if !cac.Valid(ch) {
			if soc.Valid(ch) {
				socCount++
				logger.Debug("found soc chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0))
			} else {
				invalidCount++
				logger.Warning("invalid cac/soc chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0))

				h, err := cac.DoHash(buf[swarm.SpanSize:], buf[:swarm.SpanSize])
				if err != nil {
					logger.Error(err, "cac hash")
					return
				}

				computedAddr := swarm.NewAddress(h)

				if !cac.Valid(swarm.NewChunk(computedAddr, buf)) {
					logger.Warning("computed chunk is also an invalid cac", "err", err)
					return
				}

				sharedEntry := chunkstore.RetrievalIndexItem{Address: computedAddr}
				err = store.Get(&sharedEntry)
				if err != nil {
					logger.Warning("no shared entry found")
					return
				}

				logger.Warning("retrieved chunk with shared slot", "shared_address", sharedEntry.Address, "shared_timestamp", time.Unix(int64(sharedEntry.Timestamp), 0))
			}
		}
	}

	s := time.Now()

	_ = chunkstore.IterateItems(store, func(item *chunkstore.RetrievalIndexItem) error {
		total++
		return nil
	})
	logger.Info("validation count finished", "duration", time.Since(s), "total", total)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, swarm.SocMaxChunkSize)
			for item := range iteratateItemsC {
				validChunk(item, buf[:item.Location.Length])
			}
		}()
	}

	count := 0
	_ = chunkstore.IterateItems(store, func(item *chunkstore.RetrievalIndexItem) error {
		iteratateItemsC <- item
		count++
		if count%100_000 == 0 {
			logger.Info("..still validating chunks", "count", count, "invalid", invalidCount, "soc", socCount, "total", total, "percent", fmt.Sprintf("%.2f", (float64(count)*100.0)/float64(total)))
		}
		return nil
	})

	close(iteratateItemsC)

	wg.Wait()
}

// ValidatePinCollectionChunks collects all chunk addresses that are present in a pin collection but
// are either invalid or missing altogether.
func ValidatePinCollectionChunks(ctx context.Context, basePath, pin, location string, opts *Options) error {
	logger := opts.Logger

	store, err := initStore(basePath, opts)
	if err != nil {
		return fmt.Errorf("failed creating levelDB index store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error(err, "failed closing store")
		}
	}()

	fs := &dirFS{basedir: path.Join(basePath, sharkyPath)}
	sharky, err := sharky.New(fs, sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return err
	}
	defer func() {
		if err := sharky.Close(); err != nil {
			logger.Error(err, "failed closing sharky")
		}
	}()

	logger.Info("performing chunk validation")

	pv := PinIntegrity{
		Store:  store,
		Sharky: sharky,
	}

	var (
		fileName = "address.csv"
		fileLoc  = "."
	)

	if location != "" {
		if path.Ext(location) != "" {
			fileName = path.Base(location)
		}
		fileLoc = path.Dir(location)
	}

	logger.Info("saving stats", "location", fileLoc, "name", fileName)

	location = path.Join(fileLoc, fileName)

	f, err := os.OpenFile(location, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open output file for writing: %w", err)
	}

	if _, err := f.WriteString("invalid\tmissing\ttotal\taddress\n"); err != nil {
		return fmt.Errorf("write title: %w", err)
	}

	defer f.Close()

	var ch = make(chan PinStat)
	go pv.Check(ctx, logger, pin, ch)

	for st := range ch {
		report := fmt.Sprintf("%d\t%d\t%d\t%s\n", st.Invalid, st.Missing, st.Total, st.Ref)

		if _, err := f.WriteString(report); err != nil {
			logger.Error(err, "write report line")
			break
		}
	}

	return nil
}

type PinIntegrity struct {
	Store  storage.Store
	Sharky *sharky.Store
}

type PinStat struct {
	Ref                     swarm.Address
	Total, Missing, Invalid int
}

func (p *PinIntegrity) Check(ctx context.Context, logger log.Logger, pin string, out chan PinStat) {
	var stats struct {
		total, read, invalid atomic.Int32
	}

	n := time.Now()
	defer func() {
		close(out)
		logger.Info("done", "duration", time.Since(n), "read", stats.read.Load(), "invalid", stats.invalid.Load(), "total", stats.total.Load())
	}()

	validChunk := func(item *chunkstore.RetrievalIndexItem, buf []byte) bool {
		stats.total.Add(1)

		if err := p.Sharky.Read(ctx, item.Location, buf); err != nil {
			stats.read.Add(1)
			return false
		}

		ch := swarm.NewChunk(item.Address, buf)

		if cac.Valid(ch) {
			return true
		}

		if soc.Valid(ch) {
			return true
		}

		stats.invalid.Add(1)

		return false
	}

	var pins []swarm.Address

	if pin != "" {
		addr, err := swarm.ParseHexAddress(pin)
		if err != nil {
			panic(fmt.Sprintf("parse provided pin: %s", err))
		}
		pins = append(pins, addr)
	} else {
		var err error
		pins, err = pinstore.Pins(p.Store)
		if err != nil {
			logger.Error(err, "get pins")
			return
		}
	}

	logger.Info("got a total number of pins", "size", len(pins))

	var tcount, tmicrs int64
	defer func() {
		dur := float64(tmicrs) / float64(tcount)
		logger.Info("done iterating pins", "duration", dur)
	}()

	for _, pin := range pins {
		var wg sync.WaitGroup
		var (
			total, missing, invalid atomic.Int32
		)

		iteratateItemsC := make(chan *chunkstore.RetrievalIndexItem)

		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				buf := make([]byte, swarm.SocMaxChunkSize)
				for item := range iteratateItemsC {
					if ctx.Err() != nil {
						break
					}
					if !validChunk(item, buf[:item.Location.Length]) {
						invalid.Add(1)
					}
				}
			}()
		}

		var count, micrs int64

		err := pinstore.IterateCollection(p.Store, pin, func(addr swarm.Address) (bool, error) {
			n := time.Now()

			defer func() {
				count++
				micrs += time.Since(n).Microseconds()
			}()

			total.Add(1)

			rIdx := &chunkstore.RetrievalIndexItem{Address: addr}
			if err := p.Store.Get(rIdx); err != nil {
				missing.Add(1)
			} else {
				select {
				case <-ctx.Done():
					return true, nil
				case iteratateItemsC <- rIdx:
				}
			}

			return false, nil
		})

		dur := float64(micrs) / float64(count)

		if err != nil {
			logger.Error(err, "new iteration", "pin", pin, "duration", dur)
		} else {
			logger.Info("new iteration", "pin", pin, "duration", dur)
		}

		tcount++
		tmicrs += int64(dur)

		close(iteratateItemsC)

		wg.Wait()

		select {
		case <-ctx.Done():
			logger.Info("context done")
			return
		case out <- PinStat{
			Ref:     pin,
			Total:   int(total.Load()),
			Missing: int(missing.Load()),
			Invalid: int(invalid.Load())}:
		}
	}
}

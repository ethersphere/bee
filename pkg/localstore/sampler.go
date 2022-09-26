package localstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

const sampleSize = 16

type sampleStat struct {
	TotalIterated     atomic.Int64
	NotFound          atomic.Int64
	IterationDuration atomic.Int64
	GetDuration       atomic.Int64
	HmacrDuration     atomic.Int64
}

func (s *sampleStat) String() string {
	return fmt.Sprintf(
		"Total: %d NotFound: %d Iteration Durations: %d secs GetDuration: %d secs HmacrDuration: %d",
		s.TotalIterated.Load(),
		s.NotFound.Load(),
		int64(s.IterationDuration.Load()/1000000),
		int64(s.GetDuration.Load()/1000000),
		int64(s.HmacrDuration.Load()/1000000),
	)
}

func (db *DB) ReserveSample(ctx context.Context, anchor []byte, storageDepth uint8) (storage.Sample, error) {

	g, ctx := errgroup.WithContext(ctx)
	addrChan := make(chan swarm.Address)
	var stat sampleStat
	logger := db.logger.WithName("sampler").V(1).Register()

	g.Go(func() error {
		defer close(addrChan)
		iterationStart := time.Now()
		for bin := storageDepth; bin <= swarm.MaxPO; bin++ {
			err := db.pullIndex.Iterate(func(item shed.Item) (bool, error) {
				select {
				case addrChan <- swarm.NewAddress(item.Address):
					stat.TotalIterated.Inc()
					return false, nil
				case <-ctx.Done():
					return true, ctx.Err()
				case <-db.close:
					return true, errors.New("sampler: db closed")
				}
			}, &shed.IterateOptions{
				Prefix: []byte{bin},
			})
			if err != nil {
				logger.Error(err, "sampler: failed iteration")
				return err
			}
		}
		stat.IterationDuration.Add(time.Since(iterationStart).Microseconds())
		return nil
	})

	sampleItemChan := make(chan swarm.Address)
	const workers = 4
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			hmacr := hmac.New(swarm.NewHasher, anchor)

			for addr := range addrChan {
				getStart := time.Now()
				chItem, err := db.get(ctx, storage.ModeGetSync, addr)
				stat.GetDuration.Add(time.Since(getStart).Microseconds())
				if err != nil {
					stat.NotFound.Inc()
					continue
				}

				hmacrStart := time.Now()
				hmacr.Write(chItem.Data)
				taddr := hmacr.Sum(nil)
				hmacr.Reset()
				stat.HmacrDuration.Add(time.Since(hmacrStart).Microseconds())

				select {
				case sampleItemChan <- swarm.NewAddress(taddr):
					// continue
				case <-ctx.Done():
					return ctx.Err()
				case <-db.close:
					return errors.New("sampler: db closed")
				}
			}

			return nil
		})
	}

	go func() {
		g.Wait()
		close(sampleItemChan)
	}()

	sampleItems := make([]swarm.Address, 0, sampleSize)
	// insert function will insert the new item in its correct place. If the sample
	// size goes beyond what we need we omit the last item.
	insert := func(item swarm.Address) {
		added := false
		for i, sItem := range sampleItems {
			if le(item.Bytes(), sItem.Bytes()) {
				sampleItems = append(sampleItems[:i+1], sampleItems[i:]...)
				sampleItems[i] = item
				added = true
				break
			}
		}
		if len(sampleItems) > sampleSize {
			sampleItems = sampleItems[:sampleSize]
		}
		if len(sampleItems) < sampleSize && !added {
			sampleItems = append(sampleItems, item)
		}
	}

	for item := range sampleItemChan {
		currentMaxAddr := swarm.NewAddress(make([]byte, 32))
		if len(sampleItems) > 0 {
			currentMaxAddr = sampleItems[len(sampleItems)-1]
		}
		if le(item.Bytes(), currentMaxAddr.Bytes()) || len(sampleItems) < sampleSize {
			insert(item)
		}
	}

	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	for _, s := range sampleItems {
		hasher.Write(s.Bytes())
	}
	hash := hasher.Sum(nil)

	sample := storage.Sample{
		Items: sampleItems,
		Hash:  swarm.NewAddress(hash),
	}
	logger.Info("Sampler done", "Stats", stat.String(), "Sample", sample)

	return sample, nil
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b []byte) bool {
	return bytes.Compare(a, b) == -1
}

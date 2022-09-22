package localstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

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

	sampleChan := make(chan storage.SampleItem)
	const workers = 4
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			hmacr := hmac.New(swarm.NewHasher, anchor)

			for addr := range addrChan {
				getStart := time.Now()
				ch, err := db.Get(ctx, storage.ModeGetSync, addr)
				stat.GetDuration.Add(time.Since(getStart).Microseconds())
				if err != nil {
					stat.NotFound.Inc()
					continue
				}

				hmacrStart := time.Now()
				hmacr.Write(ch.Data())
				taddr := hmacr.Sum(nil)
				hmacr.Reset()
				stat.HmacrDuration.Add(time.Since(hmacrStart).Microseconds())

				select {
				case sampleChan <- storage.SampleItem{
					Address:            addr,
					TransformedAddress: swarm.NewAddress(taddr),
				}:
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
		close(sampleChan)
	}()

	const sampleSize = 16
	samples := make([]storage.SampleItem, 0, sampleSize)
	insert := func(item storage.SampleItem) {
		added := false
		for i, sItem := range samples {
			if leq(item.TransformedAddress.Bytes(), sItem.TransformedAddress.Bytes()) {
				samples = append(samples[:i+1], samples[i:]...)
				samples[i] = item
				added = true
				break
			}
		}
		if len(samples) > sampleSize {
			samples = samples[:sampleSize]
		}
		if len(samples) < sampleSize && !added {
			samples = append(samples, item)
		}
	}

	for sample := range sampleChan {
		lowestAddr := swarm.NewAddress(make([]byte, 32))
		if len(samples) > 0 {
			lowestAddr = samples[len(samples)-1].TransformedAddress
		}
		if leq(sample.TransformedAddress.Bytes(), lowestAddr.Bytes()) || len(samples) < sampleSize {
			insert(sample)
		}
	}

	hasher := swarm.NewHasher()
	defer hasher.Reset()

	for _, s := range samples {
		hasher.Write(s.TransformedAddress.Bytes())
	}
	hash := hasher.Sum(nil)

	sample := storage.Sample{
		Items: samples,
		Hash:  swarm.NewAddress(hash),
	}
	logger.Info("Sampler done", "Stats", stat.String(), "Sample", sample)

	return sample, nil
}

func leq(a, b []byte) bool {
	return bytes.Compare(a, b) == -1
}

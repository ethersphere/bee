package localstore

import (
	"context"
	"crypto/hmac"
	"encoding/binary"
	"errors"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

func (db *DB) ReserveSample(ctx context.Context, anchor []byte, storageDepth uint8) (storage.Sample, error) {

	g, ctx := errgroup.WithContext(ctx)
	addrChan := make(chan swarm.Address)

	g.Go(func() error {
		defer close(addrChan)

		for bin := storageDepth; bin <= swarm.MaxPO; bin++ {
			err := db.pullIndex.Iterate(func(item shed.Item) (bool, error) {
				select {
				case addrChan <- swarm.NewAddress(item.Address):
					return false, nil
				case <-ctx.Done():
					return true, ctx.Err()
				case <-db.close:
					return true, nil
				}
			}, &shed.IterateOptions{
				Prefix: []byte{bin},
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	sampleChan := make(chan storage.SampleItem)
	const workers = 4
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			hmacr := hmac.New(swarm.NewHasher, anchor)

			for addr := range addrChan {
				ch, err := db.Get(ctx, storage.ModeGetSync, addr)
				if err != nil {
					continue
				}

				hmacr.Write(ch.Data())
				taddr := hmacr.Sum(nil)
				hmacr.Reset()

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
	for sample := range sampleChan {
		// if len(samples) == 16 && !leq(sample.TransformedAddress.Bytes())
		for i, item := range samples {

		}
	}
}

func leq(a, b []byte) bool {
	po := swarm.Proximity(a, b)
	i := po / 64
	c := binary.BigEndian.Uint64(a[i*64 : (i+1)*64])
	if c<<po%64>>63 == 0 {
		return true
	}
	return false
}

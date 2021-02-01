// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sequence_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/feeds/sequence"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type timeout struct {
	storage.Storer
}

var searchTimeout = 30 * time.Millisecond

// Get overrides the mock storer and introduces latency
func (t *timeout) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (swarm.Chunk, error) {
	ch, err := t.Storer.Get(ctx, mode, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			time.Sleep(searchTimeout)
		}
		return ch, err
	}
	time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
	return ch, nil
}

func BenchmarkFinder(b *testing.B) {
	for _, prefill := range []int64{1, 100, 1000, 5000} {
		storer := &timeout{mock.NewStorer()}
		topic := "testtopic"

		pk, _ := crypto.GenerateSecp256k1Key()
		signer := crypto.NewDefaultSigner(pk)

		updater, err := sequence.NewUpdater(storer, signer, topic)
		if err != nil {
			b.Fatal(err)
		}
		payload := []byte("payload")

		ctx := context.Background()

		for at := int64(0); at < prefill; at++ {
			err = updater.Update(ctx, at, payload)
			if err != nil {
				b.Fatal(err)
			}
		}
		latest := prefill
		err = updater.Update(ctx, latest, payload)
		if err != nil {
			b.Fatal(err)
		}
		now := prefill
		for k, finder := range []feeds.Lookup{
			sequence.NewFinder(storer, updater.Feed()),
			sequence.NewAsyncFinder(storer, updater.Feed()),
		} {
			names := []string{"sync", "async"}
			b.Run(fmt.Sprintf("%s:prefill=%d, latest/now=%d", names[k], prefill, now), func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					_, err := finder.At(ctx, now, 0)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

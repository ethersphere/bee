// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package epochs_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/feeds/epochs"
	feedstesting "github.com/ethersphere/bee/pkg/feeds/testing"
	"github.com/ethersphere/bee/pkg/storage/mock"
)

func BenchmarkFinder(b *testing.B) {
	for _, i := range []int{0, 8, 30} {
		for _, prefill := range []int64{1, 50} {
			after := int64(50)
			storer := &feedstesting.Timeout{Storer: mock.NewStorer()}
			topicStr := "testtopic"
			topic, err := crypto.LegacyKeccak256([]byte(topicStr))
			if err != nil {
				b.Fatal(err)
			}

			pk, _ := crypto.GenerateSecp256k1Key()
			signer := crypto.NewDefaultSigner(pk)

			updater, err := epochs.NewUpdater(storer, signer, topic)
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
			latest := after + (1 << i)
			err = updater.Update(ctx, latest, payload)
			if err != nil {
				b.Fatal(err)
			}

			for _, j := range []int64{0, 8, 30} {
				now := latest + 1<<j
				for k, finder := range []feeds.Lookup{
					epochs.NewFinder(storer, updater.Feed()),
					epochs.NewAsyncFinder(storer, updater.Feed()),
				} {
					names := []string{"sync", "async"}
					b.Run(fmt.Sprintf("%s:prefill=%d, latest=%d, now=%d", names[k], prefill, latest, now), func(b *testing.B) {
						for n := 0; n < b.N; n++ {
							_, _, _, err := finder.At(ctx, now, after)
							if err != nil {
								b.Fatal(err)
							}
						}
					})
				}
			}
		}
	}
}

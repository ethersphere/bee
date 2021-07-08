// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/pss"
)

func newTargets(length, depth int) pss.Targets {
	targets := make([]pss.Target, length)
	for i := 0; i < length; i++ {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(i))
		targets[i] = pss.Target(buf[:depth])
	}
	return pss.Targets(targets)
}

func BenchmarkWrap(b *testing.B) {
	cases := []struct {
		length int
		depth  int
	}{
		{1, 1},
		{256, 2},
		{8, 1},
		{256, 1},
		{16, 2},
		{64, 2},
		{256, 3},
		{4096, 3},
		{16384, 3},
	}
	topic := pss.NewTopic("topic")
	msg := []byte("this is my scariest")
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		b.Fatal(err)
	}
	pubkey := &key.PublicKey
	ctx := context.Background()
	for _, c := range cases {
		name := fmt.Sprintf("length:%d,depth:%d", c.length, c.depth)
		b.Run(name, func(b *testing.B) {
			targets := newTargets(c.length, c.depth)
			for i := 0; i < b.N; i++ {
				if _, err := pss.Wrap(ctx, topic, msg, pubkey, targets); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

}

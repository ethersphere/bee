// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trojan_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/trojan"
)

func newTargets(length, depth int) trojan.Targets {
	targets := make([]trojan.Target, length)
	for i := 0; i < length; i++ {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(i))
		targets[i] = trojan.Target(buf[:depth])
	}
	return trojan.Targets(targets)
}

func BenchmarkWrap(b *testing.B) {
	payload := []byte("foopayload")
	m, err := trojan.NewMessage(testTopic, payload)
	if err != nil {
		b.Fatal(err)
	}
	cases := []struct {
		length int
		depth  int
	}{
		{1, 1},
		{4, 1},
		{16, 1},
		{16, 2},
		{64, 2},
		{256, 2},
		{256, 3},
		{4096, 3},
		{16384, 3},
	}
	for _, c := range cases {
		name := fmt.Sprintf("length:%d,depth:%d", c.length, c.depth)
		b.Run(name, func(b *testing.B) {
			targets := newTargets(c.length, c.depth)
			for i := 0; i < b.N; i++ {
				if _, err := m.Wrap(context.Background(), targets); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

}

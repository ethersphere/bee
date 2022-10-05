// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package sharky_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/ethersphere/bee/pkg/sharky"
)

func TestLocationSerialization(t *testing.T) {
	t.Parallel()

	for _, tc := range []*sharky.Location{
		{
			Shard:  1,
			Slot:   100,
			Length: 4096,
		},
		{
			Shard:  0,
			Slot:   0,
			Length: 0,
		},
		{
			Shard:  math.MaxUint8,
			Slot:   math.MaxUint32,
			Length: math.MaxUint16,
		},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%d_%d_%d", tc.Shard, tc.Slot, tc.Length), func(t *testing.T) {
			t.Parallel()

			buf, err := tc.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}

			if len(buf) != sharky.LocationSize {
				t.Fatal("unexpected length of buffer")
			}

			l2 := &sharky.Location{}

			err = l2.UnmarshalBinary(buf)
			if err != nil {
				t.Fatal(err)
			}

			if l2.Shard != tc.Shard || l2.Slot != tc.Slot || l2.Length != tc.Length {
				t.Fatalf("read incorrect values from buf exp: %v found %v", tc, l2)
			}
		})
	}
}

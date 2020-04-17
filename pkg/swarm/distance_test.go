// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"testing"
)

type distanceTest struct {
	x      []byte
	y      []byte
	result string
}

type distanceCmpTest struct {
	a      []byte
	x      []byte
	y      []byte
	result int
}

var (
	distanceTests = []distanceTest{
		{
			x:      MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000").Bytes(),
			y:      MustParseHexAddress("8200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			result: "8593944123082061379093159043613555660984881674403010612303492563087302590464",
		},
	}

	distanceCmpTests = []distanceCmpTest{
		{
			a:      MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000").Bytes(), // 10010001
			x:      MustParseHexAddress("8200000000000000000000000000000000000000000000000000000000000000").Bytes(), // 10000010 xor(0x91,0x82) =	19
			y:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(), // 00010010 xor(0x91,0x12) = 131
			result: 1,
		},
		{
			a:      MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000").Bytes(),
			x:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			y:      MustParseHexAddress("8200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			result: -1,
		},
		{
			a:      MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000").Bytes(),
			x:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			y:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			result: 0,
		},
	}
)

// TestDistance tests the correctness of the distance calculation.
func TestDistance(t *testing.T) {
	for _, dt := range distanceTests {
		distance, err := Distance(dt.x, dt.y)
		if err != nil {
			t.Fatal(err)
		}
		if distance.String() != dt.result {
			t.Fatalf("incorrect distance, expected %s, got %s (x: %x, y: %x)", dt.result, distance.String(), dt.x, dt.y)
		}
	}
}

// TestDistanceCmp tests the distance comparison method.
func TestDistanceCmp(t *testing.T) {
	for _, dt := range distanceCmpTests {
		direction, err := DistanceCmp(dt.a, dt.x, dt.y)
		if err != nil {
			t.Fatal(err)
		}
		if direction != dt.result {
			t.Fatalf("incorrect distance compare, expected %d, got %d (a: %x, x: %x, y: %x)", dt.result, direction, dt.a, dt.x, dt.y)
		}
	}
}

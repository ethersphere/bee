// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"errors"
	"math/big"
)

// Distance returns the distance between address x and address y as a (comparable) big integer using the distance metric defined in the swarm specification.
// Fails if not all addresses are of equal length.
func Distance(x, y Address) (*big.Int, error) {
	distanceBytes, err := DistanceRaw(x, y)
	if err != nil {
		return nil, err
	}
	r := big.NewInt(0)
	r.SetBytes(distanceBytes)
	return r, nil
}

// DistanceRaw returns the distance between address x and address y in big-endian binary format using the distance metric defined in the swarm specification.
// Fails if not all addresses are of equal length.
func DistanceRaw(x, y Address) ([]byte, error) {
	xb, yb := x.b, y.b

	if len(xb) != len(yb) {
		return nil, errors.New("address length must match")
	}
	c := make([]byte, len(xb))
	for i, addr := range xb {
		c[i] = addr ^ yb[i]
	}
	return c, nil
}

// DistanceCmp compares x and y to a in terms of the distance metric defined in the swarm specification.
// it returns:
//   - 1 if x is closer to a than y
//   - 0 if x and y are equally far apart from a (this means that x and y are the same address)
//   - -1 if x is farther from a than y
//
// Fails if not all addresses are of equal length.
func DistanceCmp(a, x, y Address) (int, error) {
	ab, xb, yb := a.b, x.b, y.b

	if len(ab) != len(xb) || len(ab) != len(yb) {
		return 0, errors.New("address length must match")
	}

	for i := range ab {
		dx := xb[i] ^ ab[i]
		dy := yb[i] ^ ab[i]
		if dx == dy {
			continue
		} else if dx < dy {
			return 1, nil
		}
		return -1, nil
	}
	return 0, nil
}

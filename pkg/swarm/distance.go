package swarm

import (
	"errors"
	"math/big"
)

// Distance returns the distance between address x and address y as a (comparable) big integer using the distance metric defined in the swarm specification
// Fails if not all addresses are of equal length
func Distance(x, y []byte) (*big.Int, error) {
	distanceBytes, err := DistanceRaw(x, y)
	if err != nil {
		return nil, err
	}
	r := big.NewInt(0)
	r.SetBytes(distanceBytes)
	return r, nil
}

// DistanceRaw returns the distance between address x and address y in big-endian binary format using the distance metric defined in the swarm specfication
// Fails if not all addresses are of equal length
func DistanceRaw(x, y []byte) ([]byte, error) {
	if len(x) != len(y) {
		return nil, errors.New("address length must match")
	}
	c := make([]byte, len(x))
	for i, addr := range x {
		c[i] = addr ^ y[i]
	}
	return c, nil
}

// DistanceCmp compares x and y to a in terms of the distance metric defined in the swarm specfication
// it returns:
// 	1 if x is closer to a than y
// 	0 if x and y are equally far apart from (this means that x and y are the same address)
// 	-1 if x is farther from a than y
// Fails if not all addresses are of equal length
func DistanceCmp(a, x, y []byte) (int, error) {
	if len(a) != len(x) || len(a) != len(y) {
		return 0, errors.New("address length must match")
	}
	return ProxCmp(a, y, x), nil
}

// ProxCmp compares the distances x->a and y->a
// Returns -1 if x is closer to a, 1 if y is closer to a
// and 0 if they are equal.
func ProxCmp(a, x, y []byte) int {
	for i := range a {
		dx := x[i] ^ a[i]
		dy := y[i] ^ a[i]
		if dx == dy {
			continue
		} else if dx > dy {
			return 1
		}
		return -1
	}
	return 0
}

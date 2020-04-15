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
	x      []byte
	y      []byte
	z      []byte
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
			x:      MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000").Bytes(),
			y:      MustParseHexAddress("8200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			z:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			result: 1,
		},
		{
			x:      MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000").Bytes(),
			y:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			z:      MustParseHexAddress("8200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			result: -1,
		},
		{
			x:      MustParseHexAddress("9100000000000000000000000000000000000000000000000000000000000000").Bytes(),
			y:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			z:      MustParseHexAddress("1200000000000000000000000000000000000000000000000000000000000000").Bytes(),
			result: 0,
		},
	}
)

// TestDistance tests the correctness of the distance calculation
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

// TestDistanceCmp tests the distance comparison method
func TestDistanceCmp(t *testing.T) {
	for _, dt := range distanceCmpTests {
		direction, err := DistanceCmp(dt.x, dt.y, dt.z)
		if err != nil {
			t.Fatal(err)
		}
		if direction != dt.result {
			t.Fatalf("incorrect distance compare, expected %d, got %d (x: %x, y: %x, z: %x)", dt.result, direction, dt.x, dt.y, dt.z)
		}
	}
}

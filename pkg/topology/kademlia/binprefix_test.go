package kademlia

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestGenerateCommonBinPrefixes(t *testing.T) {
	base := swarm.MustParseHexAddress("abcdef1234567890")
	suffixLength := 3

	suffixLengths := make([]int, 32)
	for i := range suffixLengths {
		suffixLengths[i] = 3
	}

	prefixes := generateCommonBinPrefixes(base, suffixLengths)

	if len(prefixes) != int(swarm.MaxBins) {
		t.Fatalf("expected %d bins, got %d", swarm.MaxBins, len(prefixes))
	}

	for i := 0; i < int(swarm.MaxBins); i++ {
		for j := 0; j < len(prefixes[i]); j++ {
			proximity := swarm.Proximity(base.Bytes(), prefixes[i][j].Bytes())
			if want := uint8(i); want != proximity {
				t.Fatalf("expected proximity %d for bin %d, got %d", want, i, proximity)
			}
		}
	}

	bitCombinationsCount := 1 << suffixLength
	for i := 0; i < int(swarm.MaxBins); i++ {
		if len(prefixes[i]) != bitCombinationsCount {
			t.Fatalf("expected %d addresses in bin %d, got %d", bitCombinationsCount, i, len(prefixes[i]))
		}

		for j := 0; j < bitCombinationsCount; j++ {
			if len(prefixes[i][j].Bytes()) != len(base.Bytes()) {
				t.Fatalf("expected address length %d, got %d", len(base.Bytes()), len(prefixes[i][j].Bytes()))
			}
		}
	}
}

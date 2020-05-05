package file

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

var Spans []int64

func init() {
	Spans = GenerateSpanSizes(9)
}

// generates a dictionary of maximum span lengths per level represented by one SectionSize() of data
//
// TODO: level 0 should be SectionSize() not Branches()
func GenerateSpanSizes(levels int) []int64 {
	spans := make([]int64, levels)
	var span int64 = 1
	for i := 0; i < 9; i++ {
		spans[i] = span
		span *= swarm.Branches
	}
	return spans
}

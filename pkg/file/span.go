package file

import (
	"math"

	"github.com/ethersphere/bee/pkg/swarm"
)

var Spans []int64

func init() {
	Spans = GenerateSpanSizes(9, swarm.Branches)
}

// generates a dictionary of maximum span lengths per level represented by one SectionSize() of data
//
// TODO: level 0 should be SectionSize() not Branches()
func GenerateSpanSizes(levels, branches int) []int64 {
	spans := make([]int64, levels)
	branchesSixtyfour := int64(branches)
	var span int64 = 1
	for i := 0; i < 9; i++ {
		spans[i] = span
		span *= branchesSixtyfour
	}
	return spans
}

// getLevelsFromLength calculates the last level index which a particular data section count will result in.
// The returned level will be the level of the root hash.
func GetLevelsFromLength(length int64, sectionSize, branches int) int {
	s := int64(sectionSize)
	b := int64(branches)
	if length == 0 {
		return 0
	} else if length <= s*b {
		return 1
	}
	c := (length - 1) / s

	return int(math.Log(float64(c))/math.Log(float64(b)) + 1)
}

package sharky_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"github.com/ethersphere/bee/pkg/sharky"
)

func TestLocationSerialization(t *testing.T) {
	for _, tc := range []*sharky.Location{
		{
			Shard:  1,
			Offset: 100,
			Length: 4096,
		},
		{
			Shard:  0,
			Offset: 0,
			Length: 0,
		},
		{
			Shard:  math.MaxInt8,
			Offset: math.MaxInt64,
			Length: math.MaxInt64,
		},
	} {
		t.Run(fmt.Sprintf("%d_%d_%d", tc.Shard, tc.Offset, tc.Length), func(st *testing.T) {
			buf, err := tc.MarshalBinary()
			if err != nil {
				st.Fatal(err)
			}

			if len(buf) != 1+binary.MaxVarintLen64*2 {
				st.Fatal("unexpected length of buffer")
			}

			l2 := &sharky.Location{}

			err = l2.UnmarshalBinary(buf)
			if err != nil {
				t.Fatal(err)
			}

			if l2.Shard != tc.Shard || l2.Offset != tc.Offset || l2.Length != tc.Length {
				t.Fatalf("read incorrect values from buf exp: %v found %v", tc, l2)
			}
		})
	}
}

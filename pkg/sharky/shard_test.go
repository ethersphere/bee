package sharky_test

import (
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
			Shard: math.MaxInt8,
			// If we use offset and length as MaxInt64 we will fail as we pack
			// only 8 bytes. The values here are pretty high, higher than what makes
			// sense for our implementation and they can be packed into 8 bytes
			Offset: 35000000000000000,
			Length: 35000000000000000,
		},
	} {
		t.Run(fmt.Sprintf("%d_%d_%d", tc.Shard, tc.Offset, tc.Length), func(st *testing.T) {
			buf, err := tc.MarshalBinary()
			if err != nil {
				st.Fatal(err)
			}

			if len(buf) != sharky.LocationSize {
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

package sharky_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/sharky"
)

//TODO: Add tests for edge cases
func TestLocationSerialization(t *testing.T) {
	l := &sharky.Location{
		Shard:  1,
		Offset: 100,
		Length: 4096,
	}

	buf, err := l.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	if len(buf) != 17 {
		t.Fatal("unexpected length of buffer")
	}

	l2 := &sharky.Location{}

	err = l2.UnmarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}

	if l2.Shard != l.Shard || l2.Offset != l.Offset || l2.Length != l.Length {
		t.Fatalf("read incorrect values from buf exp: %v found %v", l, l2)
	}
}

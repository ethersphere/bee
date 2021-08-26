package pslice_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology/pslice"
)

func Benchmark2DAdd(b *testing.B) {
	var (
		base = test.RandomAddress()
		ps   = pslice.New2D(16, base)
	)

	for i := 0; i < 16; i++ {
		for j := 0; j < 1000; j++ {
			ps.Add(test.RandomAddressAt(base, i))
		}
	}

	const po = 8

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ps.Add(test.RandomAddressAt(base, po))
	}
}

func Benchmark2DAddReverse(b *testing.B) {
	var (
		base = test.RandomAddress()
		ps   = pslice.New2D(16, base)
	)

	for i := 15; i >= 0; i-- {
		for j := 0; j < 1000; j++ {
			ps.Add(test.RandomAddressAt(base, i))
		}
	}

	const po = 8

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ps.Add(test.RandomAddressAt(base, po))
	}
}

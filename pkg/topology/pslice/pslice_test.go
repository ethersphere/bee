// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pslice_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology/pslice"
)

// TestShallowestEmpty tests that ShallowestEmpty functionality works correctly.
func TestShallowestEmpty(t *testing.T) {
	var (
		base  = test.RandomAddress()
		ps    = pslice.New(16, base)
		peers = make([][]swarm.Address, 16)
	)

	for i := 0; i < 16; i++ {
		for j := 0; j < 3; j++ {
			a := test.RandomAddressAt(base, i)
			peers[i] = append(peers[i], a)
		}
	}

	for i, v := range peers {
		for _, vv := range v {
			ps.Add(vv)
		}
		sd, none := ps.ShallowestEmpty()
		if i == 15 {
			if !none {
				t.Fatal("expected last bin to be empty, thus return no empty bins true")
			}
		} else {
			if sd != uint8(i+1) {
				t.Fatalf("expected shallow empty bin to be %d but got %d", i+1, sd)
			}
			if none {
				t.Fatal("got no empty bins but wanted some")
			}
		}
	}

	// this part removes peers in certain bins and asserts
	// that the shallowest empty bin behaves correctly once the bins
	for _, tc := range []struct {
		removePo         int
		expectShallowest uint8
	}{
		{
			removePo:         3,
			expectShallowest: 3,
		}, {
			removePo:         1,
			expectShallowest: 1,
		}, {
			removePo:         10,
			expectShallowest: 1,
		}, {
			removePo:         15,
			expectShallowest: 1,
		}, {
			removePo:         14,
			expectShallowest: 1,
		}, {
			removePo:         0,
			expectShallowest: 0,
		},
	} {
		for _, v := range peers[tc.removePo] {
			ps.Remove(v)
		}
		sd, none := ps.ShallowestEmpty()
		if sd != tc.expectShallowest || none {
			t.Fatalf("empty bin mismatch got %d want %d", sd, tc.expectShallowest)
		}
	}
	ps.Add(peers[0][0])
	if sd, none := ps.ShallowestEmpty(); sd != 1 || none {
		t.Fatalf("expected bin 1 to be empty shallowest but got %d", sd)
	}
}

// TestAddRemove checks that the Add, Remove and Exists methods work as expected.
func TestAddRemove(t *testing.T) {
	var (
		base  = test.RandomAddress()
		ps    = pslice.New(4, base)
		peers = make([]swarm.Address, 8)
	)

	// 2 peers per bin
	// indexes {0,1} {2,3} {4,5} {6,7}
	for i := 0; i < 8; i += 2 {
		a := test.RandomAddressAt(base, i/2)
		peers[i] = a

		b := test.RandomAddressAt(base, i/2)
		peers[i+1] = b
	}

	// add one
	ps.Add(peers[0])
	chkLen(t, ps, 1)
	chkExists(t, ps, peers[:1]...)
	chkNotExists(t, ps, peers[1:]...)

	// check duplicates
	ps.Add(peers[0])
	chkLen(t, ps, 1)
	chkBins(t, ps, []uint{0, 1, 1, 1})
	chkExists(t, ps, peers[:1]...)
	chkNotExists(t, ps, peers[1:]...)

	// check empty
	ps.Remove(peers[0])
	chkLen(t, ps, 0)
	chkBins(t, ps, []uint{0, 0, 0, 0})
	chkNotExists(t, ps, peers...)

	// add two in bin 0
	ps.Add(peers[0])
	ps.Add(peers[1])
	chkLen(t, ps, 2)
	chkBins(t, ps, []uint{0, 2, 2, 2})
	chkExists(t, ps, peers[:2]...)
	chkNotExists(t, ps, peers[2:]...)

	ps.Add(peers[2])
	ps.Add(peers[3])
	chkLen(t, ps, 4)
	chkBins(t, ps, []uint{0, 2, 4, 4})
	chkExists(t, ps, peers[:4]...)
	chkNotExists(t, ps, peers[4:]...)

	ps.Remove(peers[1])
	chkLen(t, ps, 3)
	chkBins(t, ps, []uint{0, 1, 3, 3})
	chkExists(t, ps, peers[0], peers[2], peers[3])
	chkNotExists(t, ps, append([]swarm.Address{peers[1]}, peers[4:]...)...)

	// this should not move the last cursor
	ps.Add(peers[7])
	chkLen(t, ps, 4)
	chkBins(t, ps, []uint{0, 1, 3, 3})
	chkExists(t, ps, peers[0], peers[2], peers[3], peers[7])
	chkNotExists(t, ps, append([]swarm.Address{peers[1]}, peers[4:7]...)...)

	ps.Add(peers[5])
	chkLen(t, ps, 5)
	chkBins(t, ps, []uint{0, 1, 3, 4})
	chkExists(t, ps, peers[0], peers[2], peers[3], peers[5], peers[7])
	chkNotExists(t, ps, []swarm.Address{peers[1], peers[4], peers[6]}...)

	ps.Remove(peers[2])
	chkLen(t, ps, 4)
	chkBins(t, ps, []uint{0, 1, 2, 3})
	chkExists(t, ps, peers[0], peers[3], peers[5], peers[7])
	chkNotExists(t, ps, []swarm.Address{peers[1], peers[2], peers[4], peers[6]}...)

	p := uint8(0)
	for i := 0; i < 8; i += 2 {
		ps.Remove(peers[i])
		ps.Remove(peers[i+1])
		p++
	}

	// check empty again
	chkLen(t, ps, 0)
	chkBins(t, ps, []uint{0, 0, 0, 0})
	chkNotExists(t, ps, peers...)
}

// TestIteratorError checks that error propagation works correctly in the iterators.
func TestIteratorError(t *testing.T) {
	var (
		base = test.RandomAddress()
		ps   = pslice.New(4, base)
		a    = test.RandomAddressAt(base, 0)
		e    = errors.New("err1")
	)

	ps.Add(a)

	f := func(p swarm.Address, _ uint8) (stop, jumpToNext bool, err error) {
		return false, false, e
	}

	err := ps.EachBin(f)
	if !errors.Is(err, e) {
		t.Fatal("didnt get expected error")
	}
}

// TestIterators tests that the EachBin and EachBinRev iterators work as expected.
func TestIterators(t *testing.T) {

	base := test.RandomAddress()

	ps := pslice.New(4, base)

	peers := make([]swarm.Address, 4)
	for i := 0; i < 4; i++ {
		a := test.RandomAddressAt(base, i)
		peers[i] = a
	}

	testIterator(t, ps, false, false, 0, []swarm.Address{})
	testIteratorRev(t, ps, false, false, 0, []swarm.Address{})

	for _, v := range peers {
		ps.Add(v)
	}

	testIterator(t, ps, false, false, 4, []swarm.Address{peers[3], peers[2], peers[1], peers[0]})
	testIteratorRev(t, ps, false, false, 4, peers)

	ps.Remove(peers[2])
	testIterator(t, ps, false, false, 3, []swarm.Address{peers[3], peers[1], peers[0]})
	testIteratorRev(t, ps, false, false, 3, []swarm.Address{peers[0], peers[1], peers[3]})

	ps.Remove(peers[0])
	testIterator(t, ps, false, false, 2, []swarm.Address{peers[3], peers[1]})
	testIteratorRev(t, ps, false, false, 2, []swarm.Address{peers[1], peers[3]})

	ps.Remove(peers[3])
	testIterator(t, ps, false, false, 1, []swarm.Address{peers[1]})
	testIteratorRev(t, ps, false, false, 1, []swarm.Address{peers[1]})

	ps.Remove(peers[1])
	testIterator(t, ps, false, false, 0, []swarm.Address{})
	testIteratorRev(t, ps, false, false, 0, []swarm.Address{})
}

func TestBinPeers(t *testing.T) {

	for _, tc := range []struct {
		peersCount []int
		label      string
	}{
		{
			peersCount: []int{0, 0, 0, 0},
			label:      "bins-empty",
		},
		{
			peersCount: []int{0, 2, 0, 4},
			label:      "some-bins-empty",
		},
		{
			peersCount: []int{0, 0, 6, 0},
			label:      "some-bins-empty",
		},
		{
			peersCount: []int{3, 4, 5, 6},
			label:      "full-bins",
		},
	} {

		t.Run(tc.label, func(t *testing.T) {

			base := test.RandomAddress()

			binPeers := make([][]swarm.Address, len(tc.peersCount))

			// prepare slice
			ps := pslice.New(len(tc.peersCount), base)
			for bin, peersCount := range tc.peersCount {
				for i := 0; i < peersCount; i++ {
					peer := test.RandomAddressAt(base, bin)
					binPeers[bin] = append(binPeers[bin], peer)
					ps.Add(peer)
				}
			}

			// compare
			for bin := range tc.peersCount {
				if !isEqual(binPeers[bin], ps.BinPeers(uint8(bin))) {
					t.Fatal("peers list do not match")
				}
				if len(binPeers[bin]) != ps.BinPeersLength(uint8(bin)) {
					t.Fatal("peers list lengths do not match")
				}
			}

			// out of bound bin check
			bins := ps.BinPeers(uint8(len(tc.peersCount)))
			if bins != nil {
				t.Fatal("peers must be nil for out of bound bin")
			}
		})
	}
}

func isEqual(a, b []swarm.Address) bool {

	if len(a) != len(b) {
		return false
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].String() < a[j].String()
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].String() < b[j].String()
	})

	for i, addr := range a {
		if !b[i].Equal(addr) {
			return false
		}
	}

	return true
}

// TestIteratorsJumpStop tests that the EachBin and EachBinRev iterators jump to next bin and stop as expected.
func TestIteratorsJumpStop(t *testing.T) {
	base := test.RandomAddress()
	ps := pslice.New(4, base)

	peers := make([]swarm.Address, 12)
	j := 0
	for i := 0; i < 4; i++ {
		for ii := 0; ii < 3; ii++ {
			a := test.RandomAddressAt(base, i)
			peers[j] = a
			ps.Add(a)
			j++
		}
	}

	// check that jump to next bin works as expected
	testIterator(t, ps, true, false, 4, []swarm.Address{peers[11], peers[8], peers[5], peers[2]})
	testIteratorRev(t, ps, true, false, 4, []swarm.Address{peers[2], peers[5], peers[8], peers[11]})

	// check that the stop functionality works correctly
	testIterator(t, ps, true, true, 1, []swarm.Address{peers[11]})
	testIteratorRev(t, ps, true, true, 1, []swarm.Address{peers[2]})

}

func testIteratorRev(t *testing.T, ps *pslice.PSlice, skipNext, stop bool, iterations int, peerseq []swarm.Address) {
	t.Helper()
	i := 0
	f := func(p swarm.Address, po uint8) (bool, bool, error) {
		if i == iterations {
			t.Fatal("too many iterations!")
		}
		if !p.Equal(peerseq[i]) {
			t.Errorf("got wrong peer seq from iterator")
		}
		i++
		return stop, skipNext, nil
	}

	err := ps.EachBinRev(f)
	if err != nil {
		t.Fatal(err)
	}
}

func testIterator(t *testing.T, ps *pslice.PSlice, skipNext, stop bool, iterations int, peerseq []swarm.Address) {
	t.Helper()
	i := 0
	f := func(p swarm.Address, po uint8) (bool, bool, error) {
		if i == iterations {
			t.Fatal("too many iterations!")
		}
		if !p.Equal(peerseq[i]) {
			t.Errorf("got wrong peer seq from iterator")
		}
		i++
		return stop, skipNext, nil
	}

	err := ps.EachBin(f)
	if err != nil {
		t.Fatal(err)
	}
}

func chkLen(t *testing.T, ps *pslice.PSlice, l int) {
	t.Helper()
	if lp := ps.Length(); lp != l {
		t.Fatalf("length mismatch, want %d got %d", l, lp)
	}
}

func chkBins(t *testing.T, ps *pslice.PSlice, seq []uint) {
	t.Helper()
	pb := pslice.PSliceBins(ps)
	for i, v := range seq {
		if pb[i] != v {
			t.Fatalf("bin seq wrong, got %d want %d, index %v", pb[i], v, pb)
		}
	}
}

func chkExists(t *testing.T, ps *pslice.PSlice, addrs ...swarm.Address) {
	t.Helper()
	for _, a := range addrs {
		if !ps.Exists(a) {
			t.Fatalf("peer %s does not exist but should have", a.String())
		}
	}
}

func chkNotExists(t *testing.T, ps *pslice.PSlice, addrs ...swarm.Address) {
	t.Helper()
	for _, a := range addrs {
		if ps.Exists(a) {
			t.Fatalf("peer %s does exists but should have not", a.String())
		}
	}
}

func BenchmarkAdd(b *testing.B) {
	var (
		base = test.RandomAddress()
		ps   = pslice.New(16, base)
	)

	for i := 0; i < 16; i++ {
		for j := 0; j < 300; j++ {
			ps.Add(test.RandomAddressAt(base, i))
		}
	}

	const po = 8

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ps.Add(test.RandomAddressAt(base, po))
	}
}

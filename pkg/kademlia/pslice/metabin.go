// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pslice

import (
	"fmt"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
)

// MetaBinTree + MetaBin forms a binary tree augmented with the following properties:

// Order : first variable digit in metabin ( equals PO + 2 for root, as PO + 1 bits are determined by proximity order of bin)
// Used for keeping track of the significant bit of the metabin

// Required : number of connected peers we want for a metabin.
// It should equal nnlowwatermark for root, halved for each depth of metabins

// Unsorted : If "Required" is less than or equal to 1, we only want 1 address to connect to from the metabin,
// so we will not create further metabins, just store the known addresses in a slice

// b0: Metabin containing addresses with '0' at significant bit
// b1: Metabin containing addresses with '1' at significant bit

type MetaBinTree struct {
	order    int
	required int
	root     *MetaBin
	Logger   logging.Logger
}

type MetaBin struct {
	order    int // significant bit of current metabin
	required int // required number of connected peers

	b0 *MetaBin // containing addresses continuing with '0' in digit 'order + 1'
	b1 *MetaBin // containing addresses continuing with '1' in digit 'order + 1'

	unsorted map[string]struct{}
	Logger   logging.Logger
}

func NewTreeMap(maxBins, requires int) []*MetaBinTree {
	barr := make([]*MetaBinTree, maxBins)
	for i := 0; i < maxBins; i++ {
		barr[i] = &MetaBinTree{order: i + 1, required: requires}
	}
	return barr

}

func (b *MetaBinTree) Insert(new swarm.Address) *MetaBinTree {
	if b.root == nil {
		b.root = &MetaBin{order: b.order, required: b.required}
	}
	b.root.insert(new)
	return b
}

func (b *MetaBinTree) Remove(old swarm.Address) *MetaBinTree {
	if b.root != nil {
		b.root.remove(old)
	}
	return b
}

func (b *MetaBinTree) MetaBinSize() int {
	if b.root != nil {
		return b.root.metabinSize()
	}
	return 0
}

func (b *MetaBinTree) CompletelyNonEmpty() bool {
	if b.root != nil {
		return b.root.completelyNonEmpty()
	}
	return false

}

func (b *MetaBinTree) Print() {
	fmt.Println("\n Printing Metabin for PO: %v \n", b.order-1)
	if b.root != nil {
		b.root.print()
		return
	}
	fmt.Println("Empty MetaBinTree")
}

func (b *MetaBin) insert(new swarm.Address) {
	// If we need more than 1 connections for this metabin, we will partition this to further metabins
	if b.required > 1 {
		// If we haven't partitioned this metabin yet, let's create the partitions now with half-half of the required number of connections
		if b.b0 == nil {
			b.b0 = &MetaBin{order: b.order + 1, required: b.required / 2}
		}
		if b.b1 == nil {
			b.b1 = &MetaBin{order: b.order + 1, required: b.required / 2}
		}
		// Add the new peer to the right metabin
		if new.Get(b.order) {
			b.b1.insert(new)
		} else {
			b.b0.insert(new)
		}

		return
	}
	// At this point, it is sure we have narrowed to a metabin with a required number of connections less or equal to 1
	// So we are not going to make more in-depth metabins, we add the new peer to the unsorted list
	b.unsorted[new.String()] = struct{}{}
}

func (b *MetaBin) remove(old swarm.Address) {
	if b.required > 1 {
		if old.Get(b.order + 1) {
			b.b1.remove(old)
		} else {
			b.b0.remove(old)
		}
		return
	}
	if _, ok := b.unsorted[old.String()]; ok {
		delete(b.unsorted, old.String())
	}
}

func (b *MetaBin) metabinSize() int {
	l0 := 0
	l1 := 0
	lu := 0
	if b.b0 != nil {
		l0 = b.b0.metabinSize()
	}
	if b.b1 != nil {
		l1 = b.b1.metabinSize()
	}
	if b.unsorted != nil {
		lu = len(b.unsorted)
	}
	if (l0 > 0 || l1 > 0) && lu > 0 {
		b.Logger.Errorf("Metabin anomaly present: both metabin and unsorted list sibling has length")
		// The reason this shouldn't ever actually happen, is because we either insert to unsorted if R <= 1, or create metabins otherwise
	}
	return l0 + l1 + lu
}

func (b *MetaBin) completelyNonEmpty() bool {
	if b.required > 1 {
		return b.b0.completelyNonEmpty() && b.b1.completelyNonEmpty()
	}
	if len(b.unsorted) > 0 {
		return true
	}

	return false

}

func (b *MetaBin) print() {

	if b.required > 1 {
		fmt.Print("0")
		b.b0.print()
		fmt.Print("1")
		b.b1.print()
	}
	if b.required <= 1 {
		fmt.Println("\n %v", b.unsorted)
	}
}

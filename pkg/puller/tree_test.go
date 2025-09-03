// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package puller_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/puller"
)

func TestTreeNodePut(t *testing.T) {
	tree := puller.NewTreeNode[int](nil, nil, 0)
	keys := [][]byte{
		{0b11000000},
		{0b11100000},
		{0b00000000},
		{0b00000001},
		{0b00010001},
	}
	for i, k := range keys {
		_ = tree.Put(k, &i)
	}

	noKeyAndValue := func(c *puller.TreeNode[int]) {
		if c.V != nil || c.K != nil {
			t.Fatalf("expected no key and value")
		}
	}
	onlyRightChild := func(c *puller.TreeNode[int]) {
		if c.C[0] != nil || c.C[1] == nil {
			t.Fatalf("expected to have a right child only")
		}
	}
	onlyLeftChild := func(c *puller.TreeNode[int]) {
		if c.C[0] == nil || c.C[1] != nil {
			t.Fatalf("expected to have a left child only")
		}
	}
	noChildren := func(c *puller.TreeNode[int]) {
		if c.C[0] != nil || c.C[1] != nil {
			t.Fatalf("expected no children")
		}
	}
	bothChildren := func(c *puller.TreeNode[int]) {
		if c.C[0] == nil || c.C[1] == nil {
			t.Fatalf("expected both left and right branches to exist")
		}
	}
	keyEqualsTo := func(c *puller.TreeNode[int], k []byte) {
		if !bytes.Equal(c.K, k) {
			t.Fatalf("expected key to be %b, got %b", k, c.K)
		}
	}
	valueEqualsTo := func(c *puller.TreeNode[int], v int) {
		if c.V == nil || *c.V != v {
			t.Fatalf("expected value to be %b, got %b", v, c.V)
		}
	}
	lengthEqualsTo := func(c *puller.TreeNode[int], l uint8) {
		if c.L != l {
			t.Fatalf("expected length to be %d, got %d", l, c.L)
		}
	}

	cursor := tree
	// Simple verification that values were stored
	noKeyAndValue(cursor)
	bothChildren(cursor)
	//check right side of the tree
	cursor = tree.C[1] // prefix: 1
	noKeyAndValue(cursor)
	onlyRightChild(cursor)
	lengthEqualsTo(cursor, 1)
	cursor = cursor.C[1] // prefix 11
	noKeyAndValue(cursor)
	bothChildren(cursor)
	cursor0 := cursor.C[0]
	noChildren(cursor0)
	keyEqualsTo(cursor0, []byte{0b11000000})
	valueEqualsTo(cursor0, 0)
	lengthEqualsTo(cursor0, 3)
	cursor1 := cursor.C[1]
	noChildren(cursor1)
	keyEqualsTo(cursor1, []byte{0b11100000})
	valueEqualsTo(cursor1, 1)
	lengthEqualsTo(cursor1, 3)

	//check right left of the tree
	cursor = tree.C[0] // prefix: 0
	noKeyAndValue(cursor)
	onlyLeftChild(cursor)
	lengthEqualsTo(cursor, 1)
	cursor = cursor.C[0] // prefix: 00
	noKeyAndValue(cursor)
	onlyLeftChild(cursor)
	lengthEqualsTo(cursor, 2)
	cursor = cursor.C[0] // prefix: 000
	noKeyAndValue(cursor)
	bothChildren(cursor)
	lengthEqualsTo(cursor, 3)
	cursor1 = cursor.C[1] // prefix: 0001
	noChildren(cursor1)
	keyEqualsTo(cursor1, []byte{0b00010001})
	valueEqualsTo(cursor1, 4)
	lengthEqualsTo(cursor1, 4)
	cursor = cursor.C[0] // prefix: 0000
	noKeyAndValue(cursor)
	onlyLeftChild(cursor)
	cursor = cursor.C[0] // prefix: 00000
	noKeyAndValue(cursor)
	onlyLeftChild(cursor)
	cursor = cursor.C[0] // prefix: 000000
	noKeyAndValue(cursor)
	onlyLeftChild(cursor)
	cursor = cursor.C[0] // prefix: 0000000
	noKeyAndValue(cursor)
	bothChildren(cursor)
	cursor0 = cursor.C[0]
	noChildren(cursor0)
	keyEqualsTo(cursor0, []byte{0b00000000})
	valueEqualsTo(cursor0, 2)
	cursor1 = cursor.C[1]
	noChildren(cursor1)
	keyEqualsTo(cursor1, []byte{0b00000001})
	valueEqualsTo(cursor1, 3)
}

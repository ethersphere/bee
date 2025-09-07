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
	t.Parallel()

	noKeyAndValue := func(t *testing.T, c *puller.TreeNode[int]) {
		t.Helper()
		if c.V != nil || c.K != nil {
			t.Fatalf("expected no key and value")
		}
	}

	onlyRightChild := func(t *testing.T, c *puller.TreeNode[int]) {
		t.Helper()
		if c.C[0] != nil || c.C[1] == nil {
			t.Fatalf("expected to have a right child only")
		}
	}

	onlyLeftChild := func(t *testing.T, c *puller.TreeNode[int]) {
		t.Helper()
		if c.C[0] == nil || c.C[1] != nil {
			t.Fatalf("expected to have a left child only")
		}
	}

	noChildren := func(t *testing.T, c *puller.TreeNode[int]) {
		t.Helper()
		if c.C[0] != nil || c.C[1] != nil {
			t.Fatalf("expected no children")
		}
	}

	bothChildren := func(t *testing.T, c *puller.TreeNode[int]) {
		t.Helper()
		if c.C[0] == nil || c.C[1] == nil {
			t.Fatalf("expected both left and right branches to exist")
		}
	}

	keyEqualsTo := func(t *testing.T, c *puller.TreeNode[int], k []byte) {
		t.Helper()
		if !bytes.Equal(c.K, k) {
			t.Fatalf("expected key to be %b, got %b", k, c.K)
		}
	}

	valueEqualsTo := func(t *testing.T, c *puller.TreeNode[int], v int) {
		t.Helper()
		if c.V == nil || *c.V != v {
			t.Fatalf("expected value to be %b, got %b", v, c.V)
		}
	}

	lengthEqualsTo := func(t *testing.T, c *puller.TreeNode[int], l uint8) {
		t.Helper()
		if c.L != l {
			t.Fatalf("expected length to be %d, got %d", l, c.L)
		}
	}

	t.Run("TestWithEdgeCases", func(t *testing.T) {
		t.Parallel()

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

		cursor := tree
		// Simple verification that values were stored
		noKeyAndValue(t, cursor)
		bothChildren(t, cursor)
		//check right side of the tree
		cursor = tree.C[1] // prefix: 1
		noKeyAndValue(t, cursor)
		onlyRightChild(t, cursor)
		lengthEqualsTo(t, cursor, 1)
		cursor = cursor.C[1] // prefix 11
		noKeyAndValue(t, cursor)
		bothChildren(t, cursor)
		cursor0 := cursor.C[0]
		noChildren(t, cursor0)
		keyEqualsTo(t, cursor0, []byte{0b11000000})
		valueEqualsTo(t, cursor0, 0)
		lengthEqualsTo(t, cursor0, 3)
		cursor1 := cursor.C[1]
		noChildren(t, cursor1)
		keyEqualsTo(t, cursor1, []byte{0b11100000})
		valueEqualsTo(t, cursor1, 1)
		lengthEqualsTo(t, cursor1, 3)

		//check right left of the tree
		cursor = tree.C[0] // prefix: 0
		noKeyAndValue(t, cursor)
		onlyLeftChild(t, cursor)
		lengthEqualsTo(t, cursor, 1)
		cursor = cursor.C[0] // prefix: 00
		noKeyAndValue(t, cursor)
		onlyLeftChild(t, cursor)
		lengthEqualsTo(t, cursor, 2)
		cursor = cursor.C[0] // prefix: 000
		noKeyAndValue(t, cursor)
		bothChildren(t, cursor)
		lengthEqualsTo(t, cursor, 3)
		cursor1 = cursor.C[1] // prefix: 0001
		noChildren(t, cursor1)
		keyEqualsTo(t, cursor1, []byte{0b00010001})
		valueEqualsTo(t, cursor1, 4)
		lengthEqualsTo(t, cursor1, 4)
		cursor = cursor.C[0] // prefix: 0000
		noKeyAndValue(t, cursor)
		onlyLeftChild(t, cursor)
		cursor = cursor.C[0] // prefix: 00000
		noKeyAndValue(t, cursor)
		onlyLeftChild(t, cursor)
		cursor = cursor.C[0] // prefix: 000000
		noKeyAndValue(t, cursor)
		onlyLeftChild(t, cursor)
		cursor = cursor.C[0] // prefix: 0000000
		noKeyAndValue(t, cursor)
		bothChildren(t, cursor)
		cursor0 = cursor.C[0]
		noChildren(t, cursor0)
		keyEqualsTo(t, cursor0, []byte{0b00000000})
		valueEqualsTo(t, cursor0, 2)
		cursor1 = cursor.C[1]
		noChildren(t, cursor1)
		keyEqualsTo(t, cursor1, []byte{0b00000001})
		valueEqualsTo(t, cursor1, 3)
	})

	t.Run("TestWithStartingLevel", func(t *testing.T) {
		tree := puller.NewTreeNode[int](nil, nil, 2)
		keys := [][]byte{
			{0b00000000},
			{0b00100000},
		}
		for i, k := range keys {
			_ = tree.Put(k, &i)
		}
		cursor := tree
		noKeyAndValue(t, cursor)
		bothChildren(t, cursor)
		lengthEqualsTo(t, cursor, 2)
		cursor0 := cursor.C[0]
		keyEqualsTo(t, cursor0, keys[0])
		valueEqualsTo(t, cursor0, 0)
		noChildren(t, cursor0)
		lengthEqualsTo(t, cursor0, 3)
		cursor1 := cursor.C[1]
		keyEqualsTo(t, cursor1, keys[1])
		valueEqualsTo(t, cursor1, 1)
		noChildren(t, cursor1)
		lengthEqualsTo(t, cursor1, 3)
	})
}

func TestBinAssignment(t *testing.T) {
	t.Parallel()

	allTrue := func(t *testing.T, from uint8, arr []bool) {
		t.Helper()
		for i := from; i < uint8(len(arr)); i++ {
			if !arr[i] {
				t.Fatalf("expected true at index %d", i)
			}
		}
	}

	allFalse := func(t *testing.T, index uint8, arrs ...[]bool) {
		t.Helper()
		for i, arr := range arrs {
			if arr[index] {
				t.Fatalf("expected false at index %d for array %d", index, i)
			}
		}
	}

	t.Run("TestEmpty", func(t *testing.T) {
		t.Parallel()

		tree := puller.NewPeerTreeNode(nil, nil, 0)
		neighbors := tree.BinAssignment()
		if len(neighbors) != 0 {
			t.Errorf("expected no neighbors, got %d", len(neighbors))
		}
	})

	t.Run("TestOnePeer", func(t *testing.T) {
		t.Parallel()

		tree := puller.NewPeerTreeNode(nil, nil, 0)
		syncBins := make([]bool, 4)
		tree.Put([]byte{0b00000000}, &puller.PeerTreeNodeValue{SyncBins: syncBins})
		neighbors := tree.BinAssignment()
		if len(neighbors) != 1 {
			t.Errorf("expected one peer, got %d", len(neighbors))
		}
		allTrue(t, 0, neighbors[0].SyncBins)
	})

	t.Run("TestWithStartingLevel", func(t *testing.T) {
		t.Parallel()

		tree := puller.NewPeerTreeNode(nil, nil, 2)
		syncBins := make([]bool, 6)
		tree.Put([]byte{0b00000000}, &puller.PeerTreeNodeValue{SyncBins: syncBins})
		neighbors := tree.BinAssignment()
		if len(neighbors) != 1 {
			t.Errorf("expected one peer, got %d", len(neighbors))
		}
		allTrue(t, 2, neighbors[0].SyncBins)
		allFalse(t, 1, neighbors[0].SyncBins)
		allFalse(t, 0, neighbors[0].SyncBins)
	})

	t.Run("TestMultiplePeers", func(t *testing.T) {
		t.Parallel()

		onlyOneTrue := func(t *testing.T, index uint8, arrs ...[]bool) {
			t.Helper()
			hadOneTrue := false
			for _, arr := range arrs {
				if arr[index] {
					if hadOneTrue {
						t.Fatalf("expected only one true")
					}
					hadOneTrue = true
				}
			}
			if !hadOneTrue {
				t.Fatalf("expected exactly one true")
			}
		}

		tree := puller.NewPeerTreeNode(nil, nil, 0)
		maxBins := 9 // bin0-bin8
		sb := make([][]bool, 0, 7)
		putSyncPeer := func(key []byte) {
			syncBins := make([]bool, maxBins)
			sb = append(sb, syncBins)
			tree.Put(key, &puller.PeerTreeNodeValue{SyncBins: syncBins})
		}
		putSyncPeer([]byte{0b00000001})
		putSyncPeer([]byte{0b00000000})
		putSyncPeer([]byte{0b00010001})
		putSyncPeer([]byte{0b00010000})
		putSyncPeer([]byte{0b11100000})
		putSyncPeer([]byte{0b11000000})
		putSyncPeer([]byte{0b11110000})

		neighbors := tree.BinAssignment()
		if len(neighbors) != 7 {
			t.Errorf("expected 7 neighbors, got %d", len(neighbors))
		}

		// check bins >= ud
		allTrue(t, 8, sb[0])
		allTrue(t, 8, sb[1])
		allTrue(t, 8, sb[2])
		allTrue(t, 8, sb[3])
		allTrue(t, 4, sb[4])
		allTrue(t, 3, sb[5])
		allTrue(t, 4, sb[6])

		// check intermediate bins
		// left branch
		allFalse(t, 7, sb[0], sb[1])
		onlyOneTrue(t, 6, sb[0], sb[1])
		onlyOneTrue(t, 5, sb[0], sb[1])
		onlyOneTrue(t, 4, sb[0], sb[1])
		allFalse(t, 7, sb[2], sb[3])
		onlyOneTrue(t, 6, sb[2], sb[3])
		onlyOneTrue(t, 5, sb[2], sb[3])
		onlyOneTrue(t, 4, sb[2], sb[3])
		allFalse(t, 3, sb[0], sb[1], sb[2], sb[3])
		// right branch
		allFalse(t, 3, sb[4], sb[6])
		allFalse(t, 2, sb[4], sb[5], sb[6])
		onlyOneTrue(t, 1, sb[4], sb[5], sb[6])
		allFalse(t, 0, sb[0], sb[1], sb[2], sb[3], sb[4], sb[5], sb[6])
	})
}

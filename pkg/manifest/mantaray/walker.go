// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"bytes"
	"context"
	"errors"
	"slices"
)

// ErrSkipSubtree can be returned by a WalkNodeFunc to skip the descendants of
// the node it was called for. The walk itself does not return it.
var ErrSkipSubtree = errors.New("skip subtree")

// WalkNodeFunc is the type of the function called for each node visited
// by WalkNode.
type WalkNodeFunc func(path []byte, node *Node, err error) error

func walkNodeFnCopyBytes(path []byte, node *Node, walkFn WalkNodeFunc) error {
	return walkFn(append(path[:0:0], path...), node, nil)
}

// walkNode recursively descends path, calling walkFn. With a non-nil start,
// nodes whose path sorts before start are not passed to walkFn, and forks
// whose paths all sort before it are not loaded.
func walkNode(ctx context.Context, path, start []byte, l Loader, n *Node, walkFn WalkNodeFunc) error {
	if n.forks == nil {
		if err := n.load(ctx, l); err != nil {
			return err
		}
	}

	if start == nil || bytes.Compare(path, start) >= 0 {
		err := walkNodeFnCopyBytes(path, n, walkFn)
		if errors.Is(err, ErrSkipSubtree) {
			return nil
		}
		if err != nil {
			return err
		}
	}

	keys := make([]byte, 0, len(n.forks))
	for k := range n.forks {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		v := n.forks[k]
		nextPath := append(path[:0:0], path...)
		nextPath = append(nextPath, v.prefix...)

		nextStart := start
		if start != nil {
			common := min(len(nextPath), len(start))
			switch bytes.Compare(nextPath[:common], start[:common]) {
			case -1:
				// every path below this fork sorts before start
				continue
			case 1:
				nextStart = nil
			default:
				// one is a prefix of the other: if start is, every path below
				// sorts at or after it; if nextPath is, keep checking below
				if len(nextPath) >= len(start) {
					nextStart = nil
				}
			}
		}

		err := walkNode(ctx, nextPath, nextStart, l, v.Node, walkFn)
		if err != nil {
			return err
		}
	}

	return nil
}

// WalkNode walks the node tree structure rooted at root, calling walkFn for
// each node in the tree, including root. All errors that arise visiting nodes
// are filtered by walkFn.
func (n *Node) WalkNode(ctx context.Context, root []byte, l Loader, walkFn WalkNodeFunc) error {
	return n.WalkNodeFrom(ctx, root, nil, l, walkFn)
}

// WalkNodeFrom is like WalkNode, but visits only the nodes whose path sorts at
// or after start, in the same order. Forks whose paths all sort before start
// are not loaded, so a walk resumed deep into a large trie costs the depth of
// the trie, not every node before start. An empty start visits every node.
func (n *Node) WalkNodeFrom(ctx context.Context, root, start []byte, l Loader, walkFn WalkNodeFunc) error {
	if len(start) == 0 {
		start = nil
	}
	node, err := n.LookupNode(ctx, root, l)
	if err != nil {
		err = walkFn(root, nil, err)
	} else {
		err = walkNode(ctx, root, start, l, node, walkFn)
	}
	if errors.Is(err, ErrSkipSubtree) {
		return nil
	}
	return err
}

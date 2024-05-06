// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"context"
	"sort"
)

// WalkNodeFunc is the type of the function called for each node visited
// by WalkNode.
type WalkNodeFunc func(path []byte, node *Node, err error) error

func walkNodeFnCopyBytes(path []byte, node *Node, walkFn WalkNodeFunc) error {
	return walkFn(append(path[:0:0], path...), node, nil)
}

// walkNode recursively descends path, calling walkFn.
func walkNode(ctx context.Context, path []byte, l Loader, n *Node, walkFn WalkNodeFunc) error {
	if n.forks == nil {
		if err := n.load(ctx, l); err != nil {
			return err
		}
	}

	err := walkNodeFnCopyBytes(path, n, walkFn)
	if err != nil {
		return err
	}

	keys := make([]byte, 0, len(n.forks))
	for k := range n.forks {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, k := range keys {
		v := n.forks[k]
		nextPath := append(path[:0:0], path...)
		nextPath = append(nextPath, v.prefix...)

		err := walkNode(ctx, nextPath, l, v.Node, walkFn)
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
	node, err := n.LookupNode(ctx, root, l)
	if err != nil {
		err = walkFn(root, nil, err)
	} else {
		err = walkNode(ctx, root, l, node, walkFn)
	}
	return err
}

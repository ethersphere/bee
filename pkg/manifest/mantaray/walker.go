// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import "context"

// WalkNodeFunc is the type of the function called for each node visited
// by WalkNode.
type WalkNodeFunc func(path []byte, node *Node, err error) error

func walkNodeFnCopyBytes(ctx context.Context, path []byte, node *Node, err error, walkFn WalkNodeFunc) error {
	return walkFn(append(path[:0:0], path...), node, nil)
}

// walkNode recursively descends path, calling walkFn.
func walkNode(ctx context.Context, path []byte, l Loader, n *Node, walkFn WalkNodeFunc) error {
	if n.forks == nil {
		if err := n.load(ctx, l); err != nil {
			return err
		}
	}

	err := walkNodeFnCopyBytes(ctx, path, n, nil, walkFn)
	if err != nil {
		return err
	}

	for _, v := range n.forks {
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

// WalkFunc is the type of the function called for each file or directory
// visited by Walk.
type WalkFunc func(path []byte, isDir bool, err error) error

func walkFnCopyBytes(path []byte, isDir bool, err error, walkFn WalkFunc) error {
	return walkFn(append(path[:0:0], path...), isDir, nil)
}

// walk recursively descends path, calling walkFn.
func walk(ctx context.Context, path, prefix []byte, l Loader, n *Node, walkFn WalkFunc) error {
	if n.forks == nil {
		if err := n.load(ctx, l); err != nil {
			return err
		}
	}

	nextPath := append(path[:0:0], path...)

	for i := 0; i < len(prefix); i++ {
		if prefix[i] == PathSeparator {
			// path ends with separator
			err := walkFnCopyBytes(nextPath, true, nil, walkFn)
			if err != nil {
				return err
			}
		}
		nextPath = append(nextPath, prefix[i])
	}

	if n.IsValueType() {
		if nextPath[len(nextPath)-1] == PathSeparator {
			// path ends with separator; already reported
		} else {
			err := walkFnCopyBytes(nextPath, false, nil, walkFn)
			if err != nil {
				return err
			}
		}
	}

	if n.IsEdgeType() {
		for _, v := range n.forks {
			err := walk(ctx, nextPath, v.prefix, l, v.Node, walkFn)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Walk walks the node tree structure rooted at root, calling walkFn for
// each file or directory in the tree, including root. All errors that arise
// visiting files and directories are filtered by walkFn.
func (n *Node) Walk(ctx context.Context, root []byte, l Loader, walkFn WalkFunc) error {
	node, err := n.LookupNode(ctx, root, l)
	if err != nil {
		return walkFn(root, false, err)
	}
	return walk(ctx, root, []byte{}, l, node, walkFn)
}

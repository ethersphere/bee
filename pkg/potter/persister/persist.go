// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package persister

import (
	"context"
	"encoding"
	"io"
)

// LoadSaver to be implemented as thin wrappers around persistent key-value storage
type LoadSaver interface {
	Load(ctx context.Context, reference []byte) (data []byte, err error) // retrieve nodes for read only operations only
	Save(ctx context.Context, data []byte) (reference []byte, err error) // persists nodes out of scopc	qfor write operations
	io.Closer
}

// TreeNode is a generic interface for recursive persistable data structures
type TreeNode interface {
	Reference() []byte
	SetReference([]byte)
	Children(func(TreeNode) error) error
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// Load uses a Loader to unmarshal a tree node from a reference
func Load(ctx context.Context, ls LoadSaver, n TreeNode) error {
	b, err := ls.Load(ctx, n.Reference())
	if err != nil {
		return err
	}
	return n.UnmarshalBinary(b)
}

// Save persists a trie recursively traversing the nodes
func Save(ctx context.Context, ls LoadSaver, n TreeNode) error {
	if ref := n.Reference(); len(ref) > 0 {
		return nil
	}
	f := func(tn TreeNode) error {
		return Save(ctx, ls, tn)
	}
	if err := n.Children(f); err != nil {
		return err
	}
	bytes, err := n.MarshalBinary()
	if err != nil {
		return err
	}
	ref, err := ls.Save(ctx, bytes)
	if err != nil {
		return err
	}
	n.SetReference(ref)
	return nil
}

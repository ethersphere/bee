// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package persister

import (
	"context"
	"encoding"
	"errors"
	"io"

	"golang.org/x/sync/errgroup"
)

var (
	// ErrNoSaver saver interface not given
	ErrNoSaver = errors.New("Node is not persisted but no saver")
	// ErrNoLoader saver interface not given
	ErrNoLoader = errors.New("Node is reference but no loader")
	//
	ErrNotFound = errors.New("Node not found")
)

// Loader defines a generic interface to retrieve nodes
// from a persistent storage
// for read only operations only
type Loader interface {
	Load(ctx context.Context, reference []byte) (data []byte, err error)
}

// Saver defines a generic interface to persist nodes
// for write operations
type Saver interface {
	Save(ctx context.Context, data []byte) (reference []byte, err error)
}

// LoadSaver is a composite interface of Loader and Saver
// it is meant to be implemented as thin wrappers around persistent storage like Swarm
type LoadSaver interface {
	Loader
	Saver
	io.Closer
}

// TreeNode is a generic interface for recursive persistable data structures
type TreeNode interface {
	Reference() []byte
	SetReference([]byte)
	Children(func(TreeNode))
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// Load uses a Loader to unmarshal a tree node from a reference
func Load(ctx context.Context, n TreeNode, l Loader) error {
	if n == nil || n.Reference() == nil {
		return nil
	}
	if l == nil {
		return ErrNoLoader
	}
	b, err := l.Load(ctx, n.Reference())
	if err != nil {
		return err
	}
	return n.UnmarshalBinary(b)
}

// Save persists a trie recursively  traversing the nodes
func Save(ctx context.Context, n TreeNode, s Saver) error {
	if s == nil {
		return ErrNoSaver
	}
	return save(ctx, n, s)
}

func save(ctx context.Context, n TreeNode, s Saver) error {
	if n != nil && n.Reference() != nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	eg, ectx := errgroup.WithContext(ctx)
	n.Children(func(ch TreeNode) {
		eg.Go(func() error {
			return save(ectx, ch, s)
		})
	})
	if err := eg.Wait(); err != nil {
		return err
	}
	bytes, err := n.MarshalBinary()
	if err != nil {
		return err
	}
	ref, err := s.Save(ctx, bytes)
	if err != nil {
		return err
	}
	n.SetReference(ref)
	return nil
}

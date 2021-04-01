// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

var (
	// ErrNoSaver saver interface not given
	ErrNoSaver = errors.New("Node is not persisted but no saver")
	// ErrNoLoader saver interface not given
	ErrNoLoader = errors.New("Node is reference but no loader")
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
}

func (n *Node) load(ctx context.Context, l Loader) error {
	if n == nil || n.ref == nil {
		return nil
	}
	if l == nil {
		return ErrNoLoader
	}
	b, err := l.Load(ctx, n.ref)
	if err != nil {
		return err
	}
	return n.UnmarshalBinary(b)
}

// Save persists a trie recursively  traversing the nodes
func (n *Node) Save(ctx context.Context, s Saver) error {
	if s == nil {
		return ErrNoSaver
	}
	return n.save(ctx, s)
}

func (n *Node) save(ctx context.Context, s Saver) error {
	if n != nil && n.ref != nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	eg, ectx := errgroup.WithContext(ctx)
	for _, f := range n.forks {
		f := f
		eg.Go(func() error {
			return f.Node.save(ectx, s)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	bytes, err := n.MarshalBinary()
	if err != nil {
		return err
	}
	n.ref, err = s.Save(ctx, bytes)
	if err != nil {
		return err
	}
	n.forks = nil
	return nil
}

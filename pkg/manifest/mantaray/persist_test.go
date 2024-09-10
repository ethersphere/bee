// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/manifest/mantaray"
)

func TestPersistIdempotence(t *testing.T) {
	t.Parallel()

	n := mantaray.New()
	paths := [][]byte{
		[]byte("aa"),
		[]byte("b"),
		[]byte("aaaaaa"),
		[]byte("aaaaab"),
		[]byte("abbbb"),
		[]byte("abbba"),
		[]byte("bbbbba"),
		[]byte("bbbaaa"),
		[]byte("bbbaab"),
	}
	ctx := context.Background()
	var ls mantaray.LoadSaver = newMockLoadSaver()
	for i := 0; i < len(paths); i++ {
		c := paths[i]
		err := n.Save(ctx, ls)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		var v [32]byte
		copy(v[:], c)
		err = n.Add(ctx, c, v[:], nil, ls)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	}
	err := n.Save(ctx, ls)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	for i := 0; i < len(paths); i++ {
		c := paths[i]
		m, err := n.Lookup(ctx, c, ls)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		var v [32]byte
		copy(v[:], c)
		if !bytes.Equal(m, v[:]) {
			t.Fatalf("expected value %x, got %x", v[:], m)
		}
	}
}

func TestPersistRemove(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		toAdd    []mantaray.NodeEntry
		toRemove [][]byte
	}{
		{
			name: "simple",
			toAdd: []mantaray.NodeEntry{
				{
					Path: []byte("/"),
					Metadata: map[string]string{
						"index-document": "index.html",
					},
				},
				{
					Path: []byte("index.html"),
				},
				{
					Path: []byte("img/1.png"),
				},
				{
					Path: []byte("img/2.png"),
				},
				{
					Path: []byte("robots.txt"),
				},
			},
			toRemove: [][]byte{
				[]byte("img/2.png"),
			},
		},
	} {
		ctx := context.Background()
		var ls mantaray.LoadSaver = newMockLoadSaver()
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// add and persist
			n := mantaray.New()
			for i := 0; i < len(tc.toAdd); i++ {
				c := tc.toAdd[i].Path
				e := tc.toAdd[i].Entry
				if len(e) == 0 {
					e = append(make([]byte, 32-len(c)), c...)
				}
				m := tc.toAdd[i].Metadata
				err := n.Add(ctx, c, e, m, ls)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
			err := n.Save(ctx, ls)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			ref := n.Reference()

			// reload and remove
			nn := mantaray.NewNodeRef(ref)
			for i := 0; i < len(tc.toRemove); i++ {
				c := tc.toRemove[i]
				err := nn.Remove(ctx, c, ls)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
			err = nn.Save(ctx, ls)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			ref = nn.Reference()

			// reload and lookup removed node
			nnn := mantaray.NewNodeRef(ref)
			for i := 0; i < len(tc.toRemove); i++ {
				c := tc.toRemove[i]
				n, err = nnn.LookupNode(ctx, c, ls)
				if !errors.Is(err, mantaray.ErrNotFound) {
					t.Fatalf("expected not found error, got %v", err)
				}
			}
		})
	}
}

type addr [32]byte
type mockLoadSaver struct {
	mtx   sync.Mutex
	store map[addr][]byte
}

func newMockLoadSaver() *mockLoadSaver {
	return &mockLoadSaver{
		store: make(map[addr][]byte),
	}
}

func (m *mockLoadSaver) Save(_ context.Context, b []byte) ([]byte, error) {
	var a addr
	hasher := sha256.New()
	_, err := hasher.Write(b)
	if err != nil {
		return nil, err
	}
	copy(a[:], hasher.Sum(nil))
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.store[a] = b
	return a[:], nil
}

func (m *mockLoadSaver) Load(_ context.Context, ab []byte) ([]byte, error) {
	var a addr
	copy(a[:], ab)
	m.mtx.Lock()
	defer m.mtx.Unlock()
	b, ok := m.store[a]
	if !ok {
		return nil, mantaray.ErrNotFound
	}
	return b, nil
}

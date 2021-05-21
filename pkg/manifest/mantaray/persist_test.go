// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/manifest/mantaray"
)

func TestPersistIdempotence(t *testing.T) {
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

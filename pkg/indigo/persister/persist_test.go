// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package persister_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/indigo/persister"
)

func TestPersistIdempotence(t *testing.T) {
	var ls persister.LoadSaver = newMockLoadSaver()
	n := newMockTreeNode(depth, 1)
	ctx := context.Background()
	ref, err := persister.Save(ctx, ls, n)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	root := &mockTreeNode{ref: ref}
	sum := 1
	base := 1
	for i := 0; i < depth; i++ {
		base *= branches
		sum += base
	}
	if c := loadAndCheck(t, ls, root, 1); c != sum {
		t.Fatalf("incorrect nodecount. want 85, got %d", sum)
	}
}

const (
	branchbits = 2
	branches   = 4
	depth      = 3
)

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
		return nil, errors.New("not found")
	}
	return b, nil
}

func (m *mockLoadSaver) Close() error {
	return nil
}

type mockTreeNode struct {
	ref      []byte
	children []*mockTreeNode
	val      int
}

func newMockTreeNode(depth, val int) *mockTreeNode {
	mtn := &mockTreeNode{val: val}
	if depth == 0 {
		return mtn
	}
	val <<= branchbits
	for i := 0; i < branches; i++ {
		mtn.children = append(mtn.children, newMockTreeNode(depth-1, val+i))
	}
	return mtn
}

func (mtn *mockTreeNode) Reference() []byte {
	return mtn.ref
}

func (mtn *mockTreeNode) SetReference(b []byte) {
	mtn.ref = b
}

func (mtn *mockTreeNode) Children(f func(persister.TreeNode) error) error {
	for _, n := range mtn.children {
		if err := f(n); err != nil {
			return err
		}
	}
	return nil
}

func (mtn *mockTreeNode) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(mtn.val))
	for _, ch := range mtn.children {
		buf = append(buf, ch.Reference()...)
	}
	return buf, nil
}

func (mtn *mockTreeNode) UnmarshalBinary(buf []byte) error {
	mtn.val = int(binary.BigEndian.Uint32(buf[:4]))
	for i := branches; i < len(buf); i += 32 {
		mtn.children = append(mtn.children, &mockTreeNode{ref: buf[i : i+32]})
	}
	return nil
}

func loadAndCheck(t *testing.T, ls persister.LoadSaver, n *mockTreeNode, val int) int {
	t.Helper()
	ctx := context.Background()
	if err := persister.Load(ctx, ls, n); err != nil {
		t.Fatal(err)
	}
	if n.val != val {
		t.Fatalf("incorrect value. want %d, got %d", val, n.val)
	}
	val <<= branchbits
	c := 1
	for i, ch := range n.children {
		c += loadAndCheck(t, ls, ch, val+i)
	}
	return c
}

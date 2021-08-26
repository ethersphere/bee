// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package persister_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/indigo/persister"
)

func TestPersistIdempotence(t *testing.T) {
	n := newMockTreeNode(3, 1)
	ctx := context.Background()
	var ls persister.LoadSaver = newMockLoadSaver()
	err := persister.Save(ctx, n, ls)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	root := &mockTreeNode{ref: n.Reference()}

	loadAndCheck(t, ls, root, 1)
}

const (
	branchbits = 2
	branches   = 4
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
		return nil, persister.ErrNotFound
	}
	return b, nil
}

// Close is noop for mockLoadSaver
func (m *mockLoadSaver) Close() error {
	return nil
}

type mockTreeNode struct {
	ref      []byte
	children []*mockTreeNode
	val      int
}

func (mtn *mockTreeNode) Children(f func(persister.TreeNode)) {
	for _, ch := range mtn.children {
		f(ch)
	}
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
	return
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

func loadAndCheck(t *testing.T, ls persister.LoadSaver, n *mockTreeNode, val int) {
	t.Helper()
	ctx := context.Background()
	if err := persister.Load(ctx, n, ls); err != nil {
		t.Fatal(err)
	}
	if n.val != val {
		t.Fatalf("incorrect value. want %d, got %d", val, n.val)
	}
	val <<= branchbits
	for i, ch := range n.children {
		loadAndCheck(t, ls, ch, val+i)
	}
}

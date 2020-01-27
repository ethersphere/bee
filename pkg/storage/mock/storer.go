package mock

import (
	"context"
	"fmt"

	"github.com/janos/bee/pkg/storage"
)

type mockStorer struct {
	store map[string][]byte
}

func NewStorer() storage.Storer {
	s := &mockStorer{
		store: make(map[string][]byte),
	}

	return s
}

func (m *mockStorer) Get(ctx context.Context, addr []byte) (data []byte, err error) {
	k := fmt.Sprintf("%x", addr)
	v, has := m.store[k]
	if has {
		return v, nil
	}
	return nil, storage.ErrNotFound
}

func (m *mockStorer) Put(ctx context.Context, addr, data []byte) error {
	k := fmt.Sprintf("%x", addr)
	m.store[k] = data
	return nil
}

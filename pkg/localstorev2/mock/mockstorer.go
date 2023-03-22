// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer

import (
	"context"
	"sync"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	"github.com/ethersphere/bee/pkg/pusher"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
)

// now returns the current time.Time; used in testing.
var now = time.Now

type mockStorer struct {
	chunkStore     storage.ChunkStore
	mu             sync.Mutex
	pins           []swarm.Address
	sessionID      atomic.Uint64
	activeSessions map[uint64]*storer.SessionInfo
	chunkPushC     chan *pusher.Op
}

type putterSession struct {
	chunkStore storage.Putter
	done       func(swarm.Address) error
}

func (p *putterSession) Put(ctx context.Context, ch swarm.Chunk) error {
	return p.chunkStore.Put(ctx, ch)
}

func (p *putterSession) Done(address swarm.Address) error {
	if p.done != nil {
		return p.done(address)
	}
	return nil
}

func (p *putterSession) Cleanup() error { return nil }

// New returns a mock storer implementation that is designed to be used for the
// unit tests.
func New() *mockStorer {
	return &mockStorer{
		chunkStore:     inmemchunkstore.New(),
		chunkPushC:     make(chan *pusher.Op),
		activeSessions: make(map[uint64]*storer.SessionInfo),
	}
}

func NewWithChunkStore(cs storage.ChunkStore) *mockStorer {
	return &mockStorer{
		chunkStore:     cs,
		chunkPushC:     make(chan *pusher.Op),
		activeSessions: make(map[uint64]*storer.SessionInfo),
	}
}

func (m *mockStorer) Upload(_ context.Context, pin bool, tagID uint64) (storer.PutterSession, error) {
	return &putterSession{
		chunkStore: m.chunkStore,
		done: func(address swarm.Address) error {
			m.mu.Lock()
			defer m.mu.Unlock()

			if pin {
				m.pins = append(m.pins, address)
			}
			if session, ok := m.activeSessions[tagID]; ok {
				session.Address = address
			}
			return nil
		},
	}, nil
}

func (m *mockStorer) NewSession() (storer.SessionInfo, error) {
	session := &storer.SessionInfo{
		TagID:     m.sessionID.Inc(),
		StartedAt: now().Unix(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeSessions[session.TagID] = session

	return *session, nil
}

func (m *mockStorer) Session(tagID uint64) (storer.SessionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.activeSessions[tagID]
	if !ok {
		return storer.SessionInfo{}, storage.ErrNotFound
	}
	return *session, nil
}

func (m *mockStorer) DeleteSession(tagID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.activeSessions[tagID]; !ok {
		return storage.ErrNotFound
	}
	delete(m.activeSessions, tagID)
	return nil
}

func (m *mockStorer) ListSessions(offset, limit int) ([]storer.SessionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions := []storer.SessionInfo{}
	for _, v := range m.activeSessions {
		sessions = append(sessions, *v)
	}
	return sessions, nil
}

func (m *mockStorer) DeletePin(_ context.Context, address swarm.Address) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for idx, p := range m.pins {
		if p.Equal(address) {
			m.pins = append(m.pins[:idx], m.pins[idx+1:]...)
			break
		}
	}
	return nil
}

func (m *mockStorer) Pins() ([]swarm.Address, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pins := make([]swarm.Address, 0, len(m.pins))
	for _, p := range m.pins {
		pins = append(pins, p.Clone())
	}
	return pins, nil
}

func (m *mockStorer) HasPin(address swarm.Address) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.pins {
		if p.Equal(address) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockStorer) NewCollection(ctx context.Context) (storer.PutterSession, error) {
	return &putterSession{
		chunkStore: m.chunkStore,
		done: func(address swarm.Address) error {
			m.mu.Lock()
			defer m.mu.Unlock()

			m.pins = append(m.pins, address)
			return nil
		},
	}, nil
}

func (m *mockStorer) Lookup() storage.Getter {
	return m.chunkStore
}

func (m *mockStorer) Cache() storage.Putter {
	return m.chunkStore
}

func (m *mockStorer) DirectUpload() storer.PutterSession {
	return &putterSession{chunkStore: storage.PutterFunc(
		func(ctx context.Context, ch swarm.Chunk) error {
			op := &pusher.Op{Chunk: ch, Err: make(chan error, 1), Direct: true}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case m.chunkPushC <- op:
				return nil
			}
		}),
	}
}

func (m *mockStorer) Download(_ bool) storage.Getter {
	return m.chunkStore
}

func (m *mockStorer) PusherFeed() <-chan *pusher.Op {
	return m.chunkPushC
}

func (m *mockStorer) ChunkStore() storage.ReadOnlyChunkStore {
	return m.chunkStore
}

func (m *mockStorer) StorageRadius() uint8 { return 0 }

func (m *mockStorer) IsWithinStorageRadius(_ swarm.Address) bool { return true }

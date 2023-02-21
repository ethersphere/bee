package mockstorer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	"github.com/ethersphere/bee/pkg/pusher"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
)

var errNotImplemented = errors.New("mock storer: not implemented")

type mockStorer struct {
	chunkStore storage.ChunkStore
	mu         sync.Mutex
	pins       []swarm.Address
	sessionID  atomic.Uint64
	chunkPushC chan *pusher.Op
}

type putterSession struct {
	chunkStore storage.Putter
	addPin     func(swarm.Address) error
}

func (p *putterSession) Put(ctx context.Context, ch swarm.Chunk) error {
	return p.chunkStore.Put(ctx, ch)
}

func (p *putterSession) Done(address swarm.Address) error {
	fmt.Println("done called")
	if p.addPin != nil {
		return p.addPin(address)
	}
	fmt.Println("done called")
	return nil
}

func (p *putterSession) Cleanup() error { return nil }

func New() storer.Storer {
	return &mockStorer{
		chunkStore: inmemchunkstore.New(),
		chunkPushC: make(chan *pusher.Op),
	}
}

func (m *mockStorer) Upload(ctx context.Context, pin bool, tagID uint64) (storer.PutterSession, error) {
	ps := &putterSession{
		chunkStore: m.chunkStore,
	}
	if pin {
		ps.addPin = func(address swarm.Address) error {
			m.mu.Lock()
			defer m.mu.Unlock()

			m.pins = append(m.pins, address)
			return nil
		}
	}
	return ps, nil
}

func (m *mockStorer) NewSession() (storer.SessionInfo, error) {
	return storer.SessionInfo{
		TagID:     m.sessionID.Inc(),
		StartedAt: time.Now().Unix(),
	}, nil
}

func (m *mockStorer) GetSessionInfo(tagID uint64) (storer.SessionInfo, error) {
	if tagID > m.sessionID.Load() {
		fmt.Println("returning err not found")
		return storer.SessionInfo{}, storage.ErrNotFound
	}
	return storer.SessionInfo{
		TagID:     tagID,
		StartedAt: time.Now().Unix(),
	}, nil
}

func (m *mockStorer) DeleteSessionInfo(tagID uint64) {}

func (m *mockStorer) ListSessions(page, limit int) ([]storer.SessionInfo, error) {
	return nil, errNotImplemented
}

func (m *mockStorer) DeletePin(ctx context.Context, address swarm.Address) error {
	return errNotImplemented
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
	return nil, errNotImplemented
}

func (m *mockStorer) Lookup() storage.Getter {
	return nil
}

func (m *mockStorer) Cache() storage.Putter {
	return nil
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

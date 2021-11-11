package nested

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/ethersphere/bee/pkg/contextstore/simplestore"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/uuid"
)

var ErrStoreChange = errors.New("store collection changed")

type Store struct {
	stores   map[string]*simplestore.Store
	mtx      sync.Mutex
	revision uint // incremented every time the map changes

	base string // the base working directory
}

func Open(base string) *Store {
	s := &Store{
		stores: make(map[string]*simplestore.Store),
		base:   base,
	}

	// populate all stores in the basedir
	entries, err := os.ReadDir(base)
	if err != nil {
		panic(err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// try to open the store
		st, err := simplestore.New(filepath.Join(base, e.Name()))
		if err != nil {
			panic(err)
		}
		s.stores[e.Name()] = st
	}

	return s
}

func (s *Store) GetByName(name string) (storage.SimpleChunkStorer, func(), error) {
	// todo protection with a waitgroup
	// so that store deletion does not occur while other goroutines are Putting into the store
	// or when other consumers read, eg from the API
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if st, found := s.stores[name]; found {
		return st, st.Protect(), nil
	}
	return nil, func() {}, storage.ErrNoContext
}

func (s *Store) List() ([]string, error) {
	var stores []string
	err := s.EachStore(func(uuid string, _ storage.SimpleChunkStorer) (bool, error) {
		stores = append(stores, uuid)
		return false, nil
	})
	return stores, err
}

func (s *Store) EachStore(cb storage.EachStoreFunc) error {
	// acquire initial revision
	// if it changes - abort the process
	s.mtx.Lock()
	revision := s.revision
	s.mtx.Unlock()

	for i := 0; i < len(s.stores); i++ {
		j := 0
		var store *simplestore.Store
		var storeName string
		s.mtx.Lock()
		if revision != s.revision {
			s.mtx.Unlock()
			return ErrStoreChange
		}

		for k, v := range s.stores {
			store = v
			storeName = k
			if j == i {
				break
			}
			j++
		}
		cleanup := store.Protect()
		s.mtx.Unlock()
		// careful with optimizing on the store pointer
		// so that some address reuse bugs do not occur on the implementations
		// that rely on the fact that the addresses have to be different inside the callback!
		stop, err := cb(storeName, store)
		cleanup()
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}

	return nil
}

// New returns a new pinning context identified by a UUID.
func (s *Store) New() (string, storage.SimpleChunkStorer, func(), error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	guid := uuid.NewString()
	st, err := simplestore.New(filepath.Join(s.base, guid))
	if err != nil {
		panic(err)
	}
	s.stores[guid] = st
	s.revision++
	return guid, st, st.Protect(), nil
}

// Get a chunk by its swarm.Address. Returns the chunk associated with
// the address alongside with its postage stamp, or a storage.ErrNotFound
// if the chunk is not found. It checks all associated stores for the chunk.
func (s *Store) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	// try in all stores
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, s := range s.stores {
		if ch, err := s.Get(ctx, addr); err == nil {
			return ch, nil
		}
	}
	return nil, storage.ErrNotFound
}

// Delete a nested store with the given name.
// This is irreversible and the data will be forever gone.
// Note that there's a potential for this operation to hang
// in case that the store is protected with a long-running
// background operation.
func (s *Store) Delete(uuid string) error {
	// lock, close the file, delete the subdir
	s.mtx.Lock()
	defer s.mtx.Unlock()

	st, ok := s.stores[uuid]
	if !ok {
		return errors.New("named store not found")
	}

	// Close will wait on any pending operations
	if err := st.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}

	delete(s.stores, uuid)
	s.revision++

	if err := os.RemoveAll(path.Join(s.base, uuid)); err != nil {
		return fmt.Errorf("remove named store: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	// todo multierror
	for _, st := range s.stores {
		st.Close()
	}
	return nil
}

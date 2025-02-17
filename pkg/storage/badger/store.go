package badger

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

type Store struct {
	db *badger.DB
}

const separator = "/"

var (
	_ storage.Store = (*Store)(nil)
)

// key returns the Item identifier for the storage.
func key(item storage.Key) []byte {
	return []byte(item.Namespace() + separator + item.ID())
}

func NewStore(path string) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts = opts.WithLoggingLevel(badger.ERROR)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Get(item storage.Item) error {
	return s.db.View(func(txn *badger.Txn) error {
		val, err := txn.Get(key(item))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return storage.ErrNotFound
			}
			return err
		}

		return val.Value(func(val []byte) error {
			return item.Unmarshal(val)
		})
	})
}

func (s *Store) GetSize(k storage.Key) (int, error) {
	size := 0
	err := s.db.View(func(txn *badger.Txn) error {
		val, err := txn.Get(key(k))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return storage.ErrNotFound
			}
			return err
		}
		return val.Value(func(val []byte) error {
			size = len(val)
			return nil
		})
	})

	return size, err
}

func (s *Store) Put(item storage.Item) error {

	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key(item), value)
	})
}

func (s *Store) Delete(item storage.Item) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key(item))
	})
}

func (s *Store) Count(key storage.Key) (int, error) {
	var count int

	err := s.db.View(func(txn *badger.Txn) error {

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte(key.Namespace() + separator)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	})
	return count, err
}

func (s *Store) Has(k storage.Key) (bool, error) {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key(k))
		return err
	})

	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// TODO: Implement Iterate
func (s *Store) Iterate(q storage.Query, fn storage.IterateFn) error {
	return s.db.View(func(txn *badger.Txn) error {
		return nil
	})
}

func (s *Store) Close() error {
	return s.db.Close()
}

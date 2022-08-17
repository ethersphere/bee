package leveldbstore

import (
	"context"
	"fmt"
	"strings"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Store struct {
	DB      *leveldb.DB
	path    string
	closeLk sync.RWMutex
}

var _ storage.Store = (*Store)(nil)
var _ storage.Tx = (*transaction)(nil)

// NewDatastore returns a new datastore backed by leveldb
// for path == "", an in memory backend will be chosen (TODO)
func NewLevelDBStore(path string, opts *opt.Options) (storage.Store, error) {
	var err error
	var db *leveldb.DB

	if path == "" {
		// db, err = leveldb.Open(storage.NewMemStorage(), opts)
		panic("new leveldb store: path expected") //TODO in mem
	} else {
		db, err = leveldb.OpenFile(path, opts)
		if errors.IsCorrupted(err) && !opts.GetReadOnly() {
			db, err = leveldb.RecoverFile(path, opts)
		}
	}

	if err != nil {
		return nil, err
	}

	ds := Store{
		DB:   db,
		path: path,
	}
	return &ds, nil
}

func (s *Store) Count(key storage.Key) (int, error) {
	panic("leveldb store: count not implemented")
}

func (s *Store) Put(item storage.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing %w", err)
	}

	key := []byte(strings.Join([]string{item.Namespace(), item.ID()}, "/"))

	return s.DB.Put(key, value, &opt.WriteOptions{Sync: true})
}

func (s *Store) Get(item storage.Item) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(strings.Join([]string{item.Namespace(), item.ID()}, "/"))

	val, err := s.DB.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return storage.ErrNotFound
		}
		return err
	}

	err = item.Unmarshal(val)
	if err != nil {
		return fmt.Errorf("failed decoding value %w", err)
	}

	return nil
}

func (s *Store) Has(sKey storage.Key) (exists bool, err error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(strings.Join([]string{sKey.Namespace(), sKey.ID()}, "/"))

	return s.DB.Has(key, nil)
}

func (s *Store) GetSize(sKey storage.Key) (size int, err error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(strings.Join([]string{sKey.Namespace(), sKey.ID()}, "/"))

	val, err := s.DB.Get(key, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}

	return len(val), nil
}

func (s *Store) Delete(item storage.Key) (err error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	key := []byte(strings.Join([]string{item.Namespace(), item.ID()}, "/"))

	return s.DB.Delete(key, &opt.WriteOptions{Sync: true})
}

func (s *Store) Iterate(q storage.Query, fn storage.IterateFn) error {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	rnge := util.BytesPrefix([]byte(q.Factory().Namespace() + "/"))
	i := s.DB.NewIterator(rnge, nil)
	defer i.Release()

	for i.Next() {
		res := storage.Result{ID: string(i.Key()), Size: len(i.Value())}
		// if q.KeysOnly || q.SizeOnly {
		// 	stop, err := fn(res)
		// 	if err != nil {
		// 		return err
		// 	}
		// 	if stop {
		// 		return nil
		// 	}
		// }

		res.Entry = q.Factory()
		err := res.Entry.Unmarshal(i.Value())
		if err != nil {
			return err
		}
		stop, err := fn(res)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}

	return nil
}

func (s *Store) Close() (err error) {
	s.closeLk.Lock()
	defer s.closeLk.Unlock()
	return s.DB.Close()
}

// A leveldb transaction embedding the accessor backed by the transaction.
type transaction struct {
	st   *Store
	do   *leveldb.Batch
	undo *leveldb.Batch
}

func (t *transaction) Get(storage.Item) error {
	panic("leveldb store: get not implemented")
}

func (t *transaction) Has(storage.Key) (bool, error) {
	panic("leveldb store: batch check presence not implemented")
}

func (t *transaction) GetSize(storage.Key) (int, error) {
	panic("leveldb store: get size not implemented")
}

func (t *transaction) Iterate(storage.Query, storage.IterateFn) error {
	panic("leveldb store: iterate not implemented")
}

func (t *transaction) Count(storage.Key) (int, error) {
	panic("leveldb store: count not implemented")
}

func (t *transaction) Put(storage.Item) error {
	panic("leveldb store: batch put not implemented")
}

func (t *transaction) Delete(storage.Key) error {
	// add delete entry to do batch
	// get and add put entry to undo batch
	panic("leveldb store: batch delete not implemented")
}

func (t *transaction) Close() error {
	panic("leveldb store: batch close not implemented")
}

func (t *transaction) Commit(ctx context.Context) error {
	panic("leveldb store: batch commit not implemented")
}

func (t *transaction) Rollback(ctx context.Context) error {
	panic("leveldb store: rollback not implemented")
}

func (s *Store) NewTransaction(ctx context.Context, readOnly bool) (storage.Tx, error) {
	s.closeLk.RLock()
	defer s.closeLk.RUnlock()

	return &transaction{st: s, do: new(leveldb.Batch), undo: new(leveldb.Batch)}, nil
}

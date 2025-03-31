// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rocksdb // Consider renaming to rocksdbstore if this becomes a separate module

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/linxGnu/grocksdb"
)

const separator = "/"

// key returns the Item identifier for the RocksDB storage.
func key(item storage.Key) []byte {
	return []byte(item.Namespace() + separator + item.ID())
}

// filters is a decorator for a slice of storage.Filters
// that helps with its evaluation.
type filters []storage.Filter

// matchAny returns true if any of the filters match the item.
func (f filters) matchAny(k string, v []byte) bool {
	for _, filter := range f {
		if filter(k, v) {
			return true
		}
	}
	return false
}

// Storer returns the underlying db store.
type Storer interface {
	DB() *grocksdb.DB
}

var (
	_ Storer        = (*Store)(nil)
	_ storage.Store = (*Store)(nil)
)

type Store struct {
	db   *grocksdb.DB
	path string
}

// New returns a new store backed by RocksDB.
// If path == "", the RocksDB will run with an in-memory backend using a temporary directory.
func New(path string, opts *grocksdb.Options) (*Store, error) {
	var (
		err error
		db  *grocksdb.DB
	)

	opts = grocksdb.NewDefaultOptions()
	opts.SetCreateIfMissing(true)

	if path == "" {
		tmpDir, err := os.MkdirTemp("", "rocksdb-")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
		log.Printf("Opening RocksDB at temp dir: %s", tmpDir)
		db, err = grocksdb.OpenDb(opts, tmpDir)
		if err != nil {
			log.Printf("Failed to open RocksDB at %s: %v", tmpDir, err)
			os.RemoveAll(tmpDir)
			return nil, err
		}
		return &Store{
			db:   db,
			path: tmpDir,
		}, nil
	}

	log.Printf("Opening RocksDB at: %s", path)
	db, err = grocksdb.OpenDb(opts, path)
	if err != nil {
		return nil, err
	}

	return &Store{
		db:   db,
		path: path,
	}, nil
}

// DB implements the Storer interface.
func (s *Store) DB() *grocksdb.DB {
	return s.db
}

// Close implements the storage.Store interface.
func (s *Store) Close() error {
	s.db.Close()
	if s.path != "" && strings.HasPrefix(s.path, os.TempDir()) {
		// Remove the temporary directory if it was created by us
		if rmErr := os.RemoveAll(s.path); rmErr != nil {
			return errors.Join(nil, rmErr)
		}
	}
	return nil
}

// Get implements the storage.Store interface.
func (s *Store) Get(item storage.Item) error {
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()

	val, err := s.db.Get(ro, key(item))
	if err != nil {
		return err
	}
	defer val.Free()

	if val.Size() == 0 {
		return storage.ErrNotFound
	}

	data := make([]byte, val.Size())
	copy(data, val.Data())

	if err = item.Unmarshal(data); err != nil {
		return fmt.Errorf("failed decoding value %w", err)
	}

	return nil
}

// Has implements the storage.Store interface.
func (s *Store) Has(k storage.Key) (bool, error) {
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()

	val, err := s.db.Get(ro, key(k))
	if err != nil {
		return false, err
	}
	defer val.Free()

	return val.Size() > 0, nil
}

// GetSize implements the storage.Store interface.
func (s *Store) GetSize(k storage.Key) (int, error) {
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()

	val, err := s.db.Get(ro, key(k))
	if err != nil {
		return 0, err
	}
	defer val.Free()

	if val.Size() == 0 {
		return 0, storage.ErrNotFound
	}

	return val.Size(), nil
}

// TODO: Implement Iterate
func (s *Store) Iterate(q storage.Query, fn storage.IterateFn) error {
	return nil
}

// Count implements the storage.Store interface.
func (s *Store) Count(key storage.Key) (int, error) {
	ro := grocksdb.NewDefaultReadOptions()
	defer ro.Destroy()

	iter := s.db.NewIterator(ro)
	defer iter.Close()

	prefix := []byte(key.Namespace() + separator)
	iter.Seek(prefix)

	var c int
	for iter.ValidForPrefix(prefix) {
		c++
		iter.Next()
	}

	return c, iter.Err()
}

// Put implements the storage.Store interface.
func (s *Store) Put(item storage.Item) error {
	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}

	wo := grocksdb.NewDefaultWriteOptions()
	defer wo.Destroy()

	return s.db.Put(wo, key(item), value)
}

// Delete implements the storage.Store interface.
func (s *Store) Delete(item storage.Item) error {
	var k []byte
	if item.Namespace() == "" {
		k = []byte(item.ID())
	} else {
		k = key(item)
	}

	wo := grocksdb.NewDefaultWriteOptions()
	defer wo.Destroy()

	return s.db.Delete(wo, k)
}

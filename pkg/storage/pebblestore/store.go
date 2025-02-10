// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pebblestore

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

const separator = "/"

// key returns the Item identifier for the Pebble storage.
func key(item storage.Key) []byte {
	return []byte(item.Namespace() + separator + item.ID())
}

// filters is a decorator for a slice of storage.Filters that helps with its evaluation.
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
	DB() *pebble.DB
}

var (
	_ Storer        = (*Store)(nil)
	_ storage.Store = (*Store)(nil)
)

type Store struct {
	db   *pebble.DB
	path string
}

// upperBound returns the smallest byte slice that does not have the given prefix.
func upperBound(prefix []byte) []byte {
	ub := make([]byte, len(prefix))
	copy(ub, prefix)
	for i := len(ub) - 1; i >= 0; i-- {
		if ub[i] < 0xff {
			ub[i]++
			return ub[:i+1]
		}
	}
	return nil
}

// New returns a new store backed by Pebble DB.
// If path == "", Pebble will run with an in-memory filesystem.
func New(path string, opts *pebble.Options) (*Store, error) {
	if opts == nil {
		opts = &pebble.Options{}
	}
	if path == "" {
		// Use an in-memory filesystem.
		opts.FS = vfs.NewMem()
		// A non-empty path is still required by Pebble.
		path = "memdb"
	}
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}
	return &Store{
		db:   db,
		path: path,
	}, nil
}

// DB implements the Storer interface.
func (s *Store) DB() *pebble.DB {
	return s.db
}

// Close implements the storage.Store interface.
func (s *Store) Close() error {
	return s.db.Close()
}

// Get implements the storage.Store interface.
func (s *Store) Get(item storage.Item) error {
	val, closer, err := s.db.Get(key(item))
	if errors.Is(err, pebble.ErrNotFound) {
		return storage.ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := closer.Close(); err != nil {
		return err
	}
	if err = item.Unmarshal(val); err != nil {
		return fmt.Errorf("failed decoding value: %w", err)
	}
	return nil
}

// Has implements the storage.Store interface.
func (s *Store) Has(k storage.Key) (bool, error) {
	_, _, err := s.db.Get(key(k))
	if errors.Is(err, pebble.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetSize implements the storage.Store interface.
func (s *Store) GetSize(k storage.Key) (int, error) {
	val, _, err := s.db.Get(key(k))
	if errors.Is(err, pebble.ErrNotFound) {
		return 0, storage.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return len(val), nil
}

// Iterate implements the storage.Store interface.
func (s *Store) Iterate(q storage.Query, fn storage.IterateFn) error {
	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	var prefix string
	if q.PrefixAtStart {
		prefix = q.Factory().Namespace()
	} else {
		if q.Factory().Namespace() != "" {
			prefix = q.Factory().Namespace() + separator + q.Prefix
		} else {
			prefix = q.Prefix
		}
	}

	// Create iterator options that mimic leveldb's util.BytesPrefix.
	iterOpts := &pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: upperBound([]byte(prefix)),
	}
	iter, _ := s.db.NewIter(iterOpts)
	defer iter.Close()

	// In case of PrefixAtStart, seek to the appropriate starting point.
	if q.PrefixAtStart {
		seekKey := []byte(q.Factory().Namespace() + separator + q.Prefix)
		if !iter.SeekGE(seekKey) {
			return nil
		}
		// Move back one entry (if possible) as in the LevelDB code.
		iter.Prev()
	}

	firstSkipped := !q.SkipFirst

	// Two separate loops for ascending and descending order.
	if q.Order == storage.KeyDescendingOrder {
		if !iter.Last() {
			return nil
		}
		for {
			if !iter.Valid() {
				break
			}
			// Copy the key and value.
			nextKey := append([]byte(nil), iter.Key()...)
			nextVal := append([]byte(nil), iter.Value()...)

			// Remove the prefix.
			keyStr := strings.TrimPrefix(string(nextKey), prefix)
			if filters(q.Filters).matchAny(keyStr, nextVal) {
				if !iter.Prev() {
					break
				}
				continue
			}

			if q.SkipFirst && !firstSkipped {
				firstSkipped = true
				if !iter.Prev() {
					break
				}
				continue
			}

			var res *storage.Result
			switch q.ItemProperty {
			case storage.QueryItemID, storage.QueryItemSize:
				res = &storage.Result{ID: keyStr, Size: len(nextVal)}
			case storage.QueryItem:
				newItem := q.Factory()
				if err := newItem.Unmarshal(nextVal); err != nil {
					return fmt.Errorf("failed unmarshaling: %w", err)
				}
				res = &storage.Result{ID: keyStr, Entry: newItem}
			default:
				return fmt.Errorf("unknown object attribute type: %v", q.ItemProperty)
			}

			stop, cbErr := fn(*res)
			if cbErr != nil {
				return fmt.Errorf("iterate callback function errored: %w", cbErr)
			}
			if stop {
				break
			}
			if !iter.Prev() {
				break
			}
		}
	} else {
		if !iter.First() {
			return nil
		}
		for {
			if !iter.Valid() {
				break
			}
			nextKey := append([]byte(nil), iter.Key()...)
			nextVal := append([]byte(nil), iter.Value()...)

			keyStr := strings.TrimPrefix(string(nextKey), prefix)
			if filters(q.Filters).matchAny(keyStr, nextVal) {
				iter.Next()
				continue
			}

			if q.SkipFirst && !firstSkipped {
				firstSkipped = true
				iter.Next()
				continue
			}

			var res *storage.Result
			switch q.ItemProperty {
			case storage.QueryItemID, storage.QueryItemSize:
				res = &storage.Result{ID: keyStr, Size: len(nextVal)}
			case storage.QueryItem:
				newItem := q.Factory()
				if err := newItem.Unmarshal(nextVal); err != nil {
					return fmt.Errorf("failed unmarshaling: %w", err)
				}
				res = &storage.Result{ID: keyStr, Entry: newItem}
			default:
				return fmt.Errorf("unknown object attribute type: %v", q.ItemProperty)
			}

			stop, cbErr := fn(*res)
			if cbErr != nil {
				return fmt.Errorf("iterate callback function errored: %w", cbErr)
			}
			if stop {
				break
			}
			iter.Next()
		}
	}

	return iter.Error()
}

// Count implements the storage.Store interface.
func (s *Store) Count(key storage.Key) (int, error) {
	prefix := key.Namespace() + separator
	iterOpts := &pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: upperBound([]byte(prefix)),
	}
	iter, _ := s.db.NewIter(iterOpts)
	defer iter.Close()

	var count int
	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}
	return count, iter.Error()
}

// Put implements the storage.Store interface.
func (s *Store) Put(item storage.Item) error {
	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}
	return s.db.Set(key(item), value, &pebble.WriteOptions{})
}

// Delete implements the storage.Store interface.
func (s *Store) Delete(item storage.Item) error {
	// For backward compatibility: if there is no namespace,
	// use the ID directly.
	var k []byte
	if item.Namespace() == "" {
		k = []byte(item.ID())
	} else {
		k = key(item)
	}
	return s.db.Delete(k, &pebble.WriteOptions{})
}

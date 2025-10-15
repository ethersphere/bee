// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/armon/go-radix"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

const (
	separator = "/"
)

// Store implements an in-memory Store. We will use the hashicorp/go-radix implementation.
// This pkg provides a mutable radix which gives O(k) lookup and ordered iteration.
type Store struct {
	st *radix.Tree
	mu sync.RWMutex
}

func New() *Store {
	return &Store{st: radix.New()}
}

func key(i storage.Key) string {
	return strings.Join([]string{i.Namespace(), i.ID()}, separator)
}

func idFromKey(key, pfx string) string {
	return strings.TrimPrefix(key, pfx+separator)
}

func (s *Store) Get(i storage.Item) error {
	s.mu.RLock()
	val, found := s.st.Get(key(i))
	s.mu.RUnlock()
	if !found {
		return storage.ErrNotFound
	}

	err := i.Unmarshal(val.([]byte))
	if err != nil {
		return fmt.Errorf("failed unmarshaling item: %w", err)
	}

	return nil
}

func (s *Store) Has(k storage.Key) (bool, error) {
	s.mu.RLock()
	_, found := s.st.Get(key(k))
	s.mu.RUnlock()

	return found, nil
}

func (s *Store) GetSize(k storage.Key) (int, error) {
	s.mu.RLock()
	val, found := s.st.Get(key(k))
	s.mu.RUnlock()
	if !found {
		return 0, storage.ErrNotFound
	}

	return len(val.([]byte)), nil
}

func (s *Store) Put(i storage.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.put(i)
}

func (s *Store) put(i storage.Item) error {
	val, err := i.Marshal()
	if err != nil {
		return fmt.Errorf("failed marshaling item: %w", err)
	}

	s.st.Insert(key(i), val)
	return nil
}

func (s *Store) Delete(i storage.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delete(i)
}

func (s *Store) delete(i storage.Item) error {
	s.st.Delete(key(i))
	return nil
}

func (s *Store) Count(k storage.Key) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	s.st.WalkPrefix(k.Namespace(), func(_ string, _ interface{}) bool {
		count++
		return false
	})
	return count, nil
}

func (s *Store) Iterate(q storage.Query, fn storage.IterateFn) error {
	if err := q.Validate(); err != nil {
		return fmt.Errorf("failed iteration: %w", err)
	}

	getNext := func(k string, v interface{}) (*storage.Result, error) {
		for _, filter := range q.Filters {
			if filter(idFromKey(k, q.Factory().Namespace()), v.([]byte)) {
				return nil, nil
			}
		}
		var res *storage.Result
		switch q.ItemProperty {
		case storage.QueryItemID, storage.QueryItemSize:
			res = &storage.Result{ID: idFromKey(k, q.Factory().Namespace()), Size: len(v.([]byte))}
		case storage.QueryItem:
			newItem := q.Factory()
			err := newItem.Unmarshal(v.([]byte))
			if err != nil {
				return nil, fmt.Errorf("failed unmarshaling: %w", err)
			}
			res = &storage.Result{ID: idFromKey(k, q.Factory().Namespace()), Entry: newItem}
		}
		return res, nil
	}

	var prefix string
	if q.PrefixAtStart {
		prefix = q.Factory().Namespace()
	} else {
		prefix = q.Factory().Namespace() + separator + q.Prefix
	}

	var (
		retErr       error
		firstSkipped = !q.SkipFirst
		skipUntil    = false
	)

	s.mu.RLock()
	defer s.mu.RUnlock()

	switch q.Order {
	case storage.KeyAscendingOrder:
		s.st.WalkPrefix(prefix, func(k string, v interface{}) bool {

			if q.PrefixAtStart && !skipUntil {
				if k >= prefix+separator+q.Prefix {
					skipUntil = true
				} else {
					return false
				}
			}

			if q.SkipFirst && !firstSkipped {
				firstSkipped = true
				return false
			}

			res, err := getNext(k, v)
			if err != nil {
				retErr = errors.Join(retErr, err)
				return true
			}
			if res != nil {
				stop, err := fn(*res)
				if err != nil {
					retErr = errors.Join(retErr, fmt.Errorf("failed in iterate function: %w", err))
					return true
				}
				return stop
			}
			return false
		})
	case storage.KeyDescendingOrder:
		// currently there is no optimal way to do reverse iteration. We can efficiently do forward
		// iteration. So we have two options, first is to reduce time complexity by compromising
		// on space complexity. So we keep track of keys and values during forward iteration
		// to do a simple reverse iteration. Other option is to reduce space complexity by keeping
		// track of only keys during forward iteration, then use Get to read the value on reverse
		// iteration. This would involve additional complexity of doing a Get on reverse iteration.
		// For now, inmem implementation is not meant to work for large datasets, so first option
		// is chosen.
		results := make([]storage.Result, 0)
		s.st.WalkPrefix(prefix, func(k string, v interface{}) bool {
			res, err := getNext(k, v)
			if err != nil {
				retErr = errors.Join(retErr, err)
				return true
			}
			if res != nil {
				results = append(results, *res)
			}
			return false
		})
		if retErr != nil {
			break
		}
		for i := len(results) - 1; i >= 0; i-- {
			if q.SkipFirst && !firstSkipped {
				firstSkipped = true
				continue
			}
			stop, err := fn(results[i])
			if err != nil {
				return fmt.Errorf("failed in iterate function: %w", err)
			}
			if stop {
				break
			}
		}
	}
	return retErr
}

func (s *Store) Close() error {
	return nil
}

package inmem

import (
	"fmt"
	"strings"
	"sync"

	"github.com/armon/go-radix"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/hashicorp/go-multierror"
)

const (
	separator = "/"
)

// store implements an in-memory Store. We need lexicographic ordering of keys for the
// store. So if we use the go in-built map, we might end up complicating this implementation.
// Instead we will use the hashicorp/go-radix implementation. This pkg provides a mutable radix
// which gives O(k) lookup and ordered iteration
type store struct {
	st *radix.Tree
	mu sync.RWMutex
}

func New() storage.Store {
	return &store{st: radix.New()}
}

func getKeyString(i storage.Key) string {
	return strings.Join([]string{i.Namespace(), i.ID()}, separator)
}

func idFromKey(pfx, key string) string {
	return strings.TrimPrefix(key, pfx+separator)
}

func (s *store) Get(i storage.Item) error {
	key := getKeyString(i)

	s.mu.RLock()
	val, found := s.st.Get(key)
	s.mu.RUnlock()
	if !found {
		return storage.ErrNotFound
	}

	err := i.Unmarshal(val.([]byte))
	if err != nil {
		return fmt.Errorf("failed unmarshaling item %w", err)
	}

	return nil
}

func (s *store) Has(k storage.Key) (bool, error) {
	key := getKeyString(k)

	s.mu.RLock()
	_, found := s.st.Get(key)
	s.mu.RUnlock()

	return found, nil
}

func (s *store) GetSize(k storage.Key) (int, error) {
	key := getKeyString(k)

	s.mu.RLock()
	val, found := s.st.Get(key)
	s.mu.RUnlock()
	if !found {
		return 0, storage.ErrNotFound
	}

	return len(val.([]byte)), nil
}

func (s *store) Put(i storage.Item) error {
	key := getKeyString(i)

	val, err := i.Marshal()
	if err != nil {
		return fmt.Errorf("failed marshaling item %w", err)
	}

	s.mu.Lock()
	s.st.Insert(key, val)
	s.mu.Unlock()

	return nil
}

func (s *store) Delete(k storage.Key) error {
	key := getKeyString(k)

	s.mu.Lock()
	_, deleted := s.st.Delete(key)
	s.mu.Unlock()
	if !deleted {
		return storage.ErrNotFound
	}
	return nil
}

func (s *store) Count(k storage.Key) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	s.st.WalkPrefix(k.Namespace(), func(_ string, _ interface{}) bool {
		count++
		return false
	})
	return count, nil
}

func (s *store) Iterate(q storage.Query, fn storage.IterateFn) error {
	if err := q.Validate(); err != nil {
		return fmt.Errorf("invalid query %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var retErr *multierror.Error

	getNext := func(k string, v interface{}) (*storage.Result, error) {
		for _, filter := range q.Filters {
			if filter(idFromKey(q.Factory().Namespace(), k), v.([]byte)) {
				return nil, nil
			}
		}
		var res *storage.Result
		switch q.ItemAttribute {
		case storage.QueryItemID, storage.QueryItemSize:
			res = &storage.Result{ID: idFromKey(q.Factory().Namespace(), k), Size: len(v.([]byte))}
		case storage.QueryItem:
			newItem := q.Factory()
			err := newItem.Unmarshal(v.([]byte))
			if err != nil {
				return nil, fmt.Errorf("failed unmarshaling %w", err)
			}
			res = &storage.Result{Entry: newItem}
		}
		return res, nil
	}

	prefix := q.Factory().Namespace()
	switch q.Order {
	case storage.KeyAscendingOrder:
		s.st.WalkPrefix(prefix, func(k string, v interface{}) bool {
			res, err := getNext(k, v)
			if err != nil {
				retErr = multierror.Append(retErr, err)
				return true
			}
			if res != nil {
				stop, err := fn(*res)
				if err != nil {
					retErr = multierror.Append(retErr, fmt.Errorf("failed in iteration %w", err))
					return true
				}
				return stop
			}
			return false
		})
	case storage.KeyDescendingOrder:
		results := make([]storage.Result, 0)
		s.st.WalkPrefix(prefix, func(k string, v interface{}) bool {
			res, err := getNext(k, v)
			if err != nil {
				retErr = multierror.Append(retErr, err)
				return true
			}
			if res != nil {
				results = append(results, *res)
			}
			return false
		})
		if retErr.ErrorOrNil() != nil {
			break
		}
		for i := len(results) - 1; i >= 0; i-- {
			stop, err := fn(results[i])
			if err != nil {
				return fmt.Errorf("failed in iteration %w", err)
			}
			if stop {
				break
			}
		}
	}
	return retErr.ErrorOrNil()
}

func (s *store) Close() error {
	return nil
}

// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

// StateIterFunc is used when iterating through StateStorer key/value pairs
type StateIterFunc func(key, val []byte) (stop bool, err error)

// StateStorer is a storage interface for storing and retrieving key/value pairs.
type StateStorer interface {
	io.Closer

	// Get unmarshalls object with the given key into the given obj.
	Get(key string, obj interface{}) error

	// Put inserts or updates the given obj stored under the given key.
	Put(key string, obj interface{}) error

	// Delete removes object form the store stored under the given key.
	Delete(key string) error

	// Iterate iterates over all keys with the given prefix and calls iterFunc.
	Iterate(prefix string, iterFunc StateIterFunc) error
}

// StateStorerCleaner is the interface for cleaning the store.
type StateStorerCleaner interface {
	// Nuke the store so that only the bare essential entries are left.
	Nuke(forgetStamps bool) error
}

// stateStoreNamespace is the namespace used for state storage.
const stateStoreNamespace = "statestore"

var _ Item = (*proxyItem)(nil)

// proxyItem is a proxy object that implements the Item interface.
// It is an intermediary object between StateStorer and Store interfaces calls.
type proxyItem struct {
	ns  string
	key string
	obj interface{}
}

// ID implements Item interface.
func (pi *proxyItem) ID() string {
	return pi.key
}

// Namespace implements Item interface.
func (pi *proxyItem) Namespace() string {
	return pi.ns
}

// Marshal implements Item interface.
func (pi *proxyItem) Marshal() ([]byte, error) {
	if pi == nil || pi.obj == nil {
		return nil, nil
	}

	switch m := pi.obj.(type) {
	case encoding.BinaryMarshaler:
		return m.MarshalBinary()
	case Marshaler:
		return m.Marshal()
	}
	return json.Marshal(pi.obj)
}

// Unmarshal implements Item interface.
func (pi *proxyItem) Unmarshal(data []byte) error {
	if pi == nil || pi.obj == nil {
		return nil
	}

	switch m := pi.obj.(type) {
	case encoding.BinaryUnmarshaler:
		return m.UnmarshalBinary(data)
	case Unmarshaler:
		return m.Unmarshal(data)
	}
	return json.Unmarshal(data, &pi.obj)
}

// Clone implements Item interface.
func (pi *proxyItem) Clone() Item {
	if pi == nil {
		return nil
	}

	obj := pi.obj
	if cloner, ok := pi.obj.(Cloner); ok {
		obj = cloner.Clone()
	}
	return &proxyItem{
		ns:  pi.ns,
		key: pi.key,
		obj: obj,
	}
}

// String implements Item interface.
func (pi proxyItem) String() string {
	return path.Join(pi.Namespace(), pi.ID())
}

// newItemProxy creates a new proxyItem.
func newItemProxy(key string, obj interface{}) *proxyItem {
	return &proxyItem{ns: stateStoreNamespace, key: key, obj: obj}
}

var _ Item = (*rawItem)(nil)

// rawItem is a proxy object that implements the Item interface.
// It is an intermediary object between StateStorer and Store iterator calls.
type rawItem struct {
	*proxyItem
}

// Marshal implements Item interface.
func (ri *rawItem) Marshal() ([]byte, error) {
	if ri == nil || ri.proxyItem == nil || ri.proxyItem.obj == nil {
		return nil, nil
	}

	if buf, ok := ri.proxyItem.obj.([]byte); ok {
		return buf, nil
	}

	return ri.proxyItem.Marshal()
}

// Unmarshal implements Item interface.
func (ri *rawItem) Unmarshal(data []byte) error {
	if ri == nil || ri.proxyItem == nil || ri.proxyItem.obj == nil || len(data) == 0 {
		return nil
	}

	if buf, ok := ri.proxyItem.obj.([]byte); ok {
		ri.proxyItem.obj = append(buf[:0], data...)
		return nil
	}

	return ri.proxyItem.Unmarshal(data)
}

var _ StateStorer = (*StateStorerAdapter)(nil)

// StateStorerAdapter is an adapter from Store to the StateStorer.
type StateStorerAdapter struct {
	storage Store
}

// Close implements StateStorer interface.
func (s *StateStorerAdapter) Close() error {
	return s.storage.Close()
}

// Get implements StateStorer interface.
func (s *StateStorerAdapter) Get(key string, obj interface{}) (err error) {
	return s.storage.Get(newItemProxy(key, obj))
}

// Put implements StateStorer interface.
func (s *StateStorerAdapter) Put(key string, obj interface{}) (err error) {
	return s.storage.Put(newItemProxy(key, obj))
}

// Delete implements StateStorer interface.
func (s *StateStorerAdapter) Delete(key string) (err error) {
	return s.storage.Delete(newItemProxy(key, nil))
}

// Iterate implements StateStorer interface.
func (s *StateStorerAdapter) Iterate(prefix string, iterFunc StateIterFunc) (err error) {
	return s.storage.Iterate(
		Query{
			Factory: func() Item { return &rawItem{newItemProxy("", []byte(nil))} },
			Prefix:  prefix,
		},
		func(res Result) (stop bool, err error) {
			key := []byte(prefix + res.ID)
			val, err := res.Entry.(*rawItem).Marshal()
			if err != nil {
				return false, err
			}
			return iterFunc(key, val)
		},
	)
}

func (s *StateStorerAdapter) Nuke(forgetStamps bool) error {
	var (
		keys               []string
		prefixesToPreserve = []string{"accounting", "pseudosettle", "swap", "non-mineable-overlay", "overlayV2_nonce"}
		err                error
	)

	if !forgetStamps {
		prefixesToPreserve = append(prefixesToPreserve, "postage")
	}

	keys, err = s.collectKeysExcept(prefixesToPreserve)
	if err != nil {
		return fmt.Errorf("collect keys except: %w", err)
	}
	return s.deleteKeys(keys)
}

func (s *StateStorerAdapter) collectKeysExcept(prefixesToPreserve []string) (keys []string, err error) {
	if err := s.Iterate("", func(k, v []byte) (bool, error) {
		stk := string(k)
		has := false
		for _, v := range prefixesToPreserve {
			if strings.HasPrefix(stk, v) {
				has = true
				break
			}
		}
		if !has {
			keys = append(keys, stk)
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *StateStorerAdapter) deleteKeys(keys []string) error {
	for _, v := range keys {
		err := s.Delete(v)
		if err != nil {
			return fmt.Errorf("deleting key %s: %w", v, err)
		}
	}
	return nil
}

// NewStateStorerAdapter creates a new StateStorerAdapter.
func NewStateStorerAdapter(storage Store) *StateStorerAdapter {
	return &StateStorerAdapter{storage: storage}
}

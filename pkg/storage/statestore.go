// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"encoding/json"
	"io"
	"path"
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
func (ip *proxyItem) ID() string {
	return ip.key
}

// Namespace implements Item interface.
func (ip *proxyItem) Namespace() string {
	return ip.ns
}

// Marshal implements Item interface.
func (ip *proxyItem) Marshal() ([]byte, error) {
	return json.Marshal(ip.obj)
}

// Unmarshal implements Item interface.
func (ip *proxyItem) Unmarshal(data []byte) error {
	return json.Unmarshal(data, &ip.obj)
}

// Clone implements Item interface.
func (ip *proxyItem) Clone() Item {
	if ip == nil {
		return nil
	}

	obj := ip.obj
	if cloner, ok := ip.obj.(Cloner); ok {
		obj = cloner.Clone()
	}
	return &proxyItem{
		ns:  ip.ns,
		key: ip.key,
		obj: obj,
	}
}

// String implements Item interface.
func (ip proxyItem) String() string {
	return path.Join(ip.Namespace(), ip.ID())
}

// newItemProxy creates a new proxyItem.
func newItemProxy(key string, obj interface{}) *proxyItem {
	return &proxyItem{ns: stateStoreNamespace, key: key, obj: obj}
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
			Factory: func() Item { return newItemProxy("", nil) },
			Prefix:  prefix,
		},
		func(res Result) (stop bool, err error) {
			key := []byte(prefix + res.ID)
			val, err := res.Entry.(*proxyItem).Marshal()
			if err != nil {
				return false, err
			}
			return iterFunc(key, val)
		},
	)
}

// NewStateStorerAdapter creates a new StateStorerAdapter.
func NewStateStorerAdapter(storage Store) *StateStorerAdapter {
	return &StateStorerAdapter{storage: storage}
}
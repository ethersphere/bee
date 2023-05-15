// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"encoding/json"
	"path"

	"github.com/ethersphere/bee/pkg/storage"
)

// stateStoreNamespace is the namespace used for state storage.
const stateStoreNamespace = "ss"

var _ storage.Item = (*proxyItem)(nil)

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
func (ip *proxyItem) Clone() storage.Item {
	if ip == nil {
		return nil
	}

	obj := ip.obj
	if cloner, ok := ip.obj.(storage.Cloner); ok {
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

var _ storage.StateStorer = (*StateStorerAdapter)(nil)

// StateStorerAdapter is an adapter from Store to the StateStorer.
type StateStorerAdapter struct {
	storage storage.Store
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
func (s *StateStorerAdapter) Iterate(prefix string, iterFunc storage.StateIterFunc) (err error) {
	return s.storage.Iterate(
		storage.Query{
			Factory: func() storage.Item { return newItemProxy("", nil) },
			Prefix:  prefix,
		},
		func(res storage.Result) (stop bool, err error) {
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
func NewStateStorerAdapter(storage storage.Store) *StateStorerAdapter {
	return &StateStorerAdapter{storage: storage}
}

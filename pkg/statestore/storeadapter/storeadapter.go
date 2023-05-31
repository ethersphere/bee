// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"encoding"
	"encoding/json"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/migration"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
)

// stateStoreNamespace is the namespace used for state storage.
const stateStoreNamespace = "statestore"

var _ storage.Item = (*proxyItem)(nil)

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
	case storage.Marshaler:
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
	case storage.Unmarshaler:
		return m.Unmarshal(data)
	}
	return json.Unmarshal(data, &pi.obj)
}

// Clone implements Item interface.
func (pi *proxyItem) Clone() storage.Item {
	if pi == nil {
		return nil
	}

	obj := pi.obj
	if cloner, ok := pi.obj.(storage.Cloner); ok {
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
	return storageutil.JoinFields(pi.Namespace(), pi.ID())
}

// newItemProxy creates a new proxyItem.
func newItemProxy(key string, obj interface{}) *proxyItem {
	return &proxyItem{ns: stateStoreNamespace, key: key, obj: obj}
}

var _ storage.Item = (*rawItem)(nil)

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
			Factory: func() storage.Item { return &rawItem{newItemProxy("", []byte(nil))} },
			Prefix:  prefix,
		},
		func(res storage.Result) (stop bool, err error) {
			key := []byte(prefix + res.ID)
			val, err := res.Entry.(*rawItem).Marshal()
			if err != nil {
				return false, err
			}
			return iterFunc(key, val)
		},
	)
}

// NewStateStorerAdapter creates a new StateStorerAdapter.
func NewStateStorerAdapter(storage storage.Store) (*StateStorerAdapter, error) {
	err := migration.Migrate(storage, AllSteps())
	if err != nil {
		return nil, err
	}
	return &StateStorerAdapter{storage: storage}, nil
}

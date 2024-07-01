// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"encoding"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
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

// newProxyItem creates a new proxyItem.
func newProxyItem(key string, obj interface{}) *proxyItem {
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

var (
	_ storage.StateStorer        = (*StateStorerAdapter)(nil)
	_ storage.StateStorerCleaner = (*StateStorerAdapter)(nil)
)

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
	return s.storage.Get(newProxyItem(key, obj))
}

// Put implements StateStorer interface.
func (s *StateStorerAdapter) Put(key string, obj interface{}) (err error) {
	return s.storage.Put(newProxyItem(key, obj))
}

// Delete implements StateStorer interface.
func (s *StateStorerAdapter) Delete(key string) (err error) {
	return s.storage.Delete(newProxyItem(key, nil))
}

// Iterate implements StateStorer interface.
func (s *StateStorerAdapter) Iterate(prefix string, iterFunc storage.StateIterFunc) (err error) {
	return s.storage.Iterate(
		storage.Query{
			Factory: func() storage.Item { return &rawItem{newProxyItem("", []byte(nil))} },
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

func (s *StateStorerAdapter) Nuke() error {
	var (
		prefixesToPreserve = []string{
			"non-mineable-overlay",
			"overlayV2_nonce",
			"pseudosettle",
			"accounting",
			"swap",
		}
		keys []string
		err  error
	)

	keys, err = s.collectKeysExcept(prefixesToPreserve)
	if err != nil {
		return fmt.Errorf("collect keys except: %w", err)
	}
	return s.deleteKeys(keys)
}

func (s *StateStorerAdapter) ClearForHopping() error {
	var (
		prefixesToPreserve = []string{
			"swap_chequebook", // to not redeploy chequebook contract
			"batchstore",      // avoid unnecessary syncing
			"transaction",     // to not resync blockchain transactions
		}
		keys []string
		err  error
	)

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
func NewStateStorerAdapter(storage storage.Store) (*StateStorerAdapter, error) {
	err := migration.Migrate(storage, "migration", allSteps(storage))
	if err != nil {
		return nil, err
	}
	return &StateStorerAdapter{storage: storage}, nil
}

package transaction

import (
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
)

type memcache struct {
	storage.IndexStore
	data map[string][]byte
}

func NewMemCache(st storage.IndexStore) *memcache {
	return &memcache{st, make(map[string][]byte)}
}

func (m *memcache) Get(i storage.Item) error {
	if val, ok := m.data[key(i)]; ok {
		return i.Unmarshal(val)
	}

	if err := m.IndexStore.Get(i); err != nil {
		return err
	}

	m.add(i)

	return nil
}

func (m *memcache) Has(k storage.Key) (bool, error) {
	if _, ok := m.data[key(k)]; ok {
		return true, nil
	}
	return m.IndexStore.Has(k)
}

func (m *memcache) Put(i storage.Item) error {
	m.add(i)
	return m.IndexStore.Put(i)
}

func (m *memcache) Delete(i storage.Item) error {
	delete(m.data, key(i))
	return m.IndexStore.Delete(i)
}

// key returns a string representation of the given key.
func key(key storage.Key) string {
	return storageutil.JoinFields(key.Namespace(), key.ID())
}

// add caches given item.
func (m *memcache) add(i storage.Item) {
	b, err := i.Marshal()
	if err != nil {
		return
	}

	m.data[key(i)] = b
}

func (m *memcache) Close() error {
	return nil
}

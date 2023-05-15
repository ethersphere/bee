package storeadapter

import (
	"strings"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/migration"
)

func AllSteps() migration.Steps {
	return map[uint64]migration.StepFn{
		1: epochMigration,
	}
}

type migrationItem struct {
	ns    string
	key   string
	value []byte
}

func (m *migrationItem) Namespace() string { return m.ns }

func (m *migrationItem) ID() string { return m.key }

func (m *migrationItem) Marshal() ([]byte, error) { return m.value, nil }

func (m *migrationItem) Unmarshal(data []byte) error {
	m.value = data
	return nil
}

func (m *migrationItem) String() string { return m.key }

func (m *migrationItem) Clone() storage.Item {
	cpyBuf := make([]byte, len(m.value))
	copy(cpyBuf, m.value)
	return &migrationItem{
		ns:    m.ns,
		key:   m.key,
		value: cpyBuf,
	}
}

func epochMigration(s storage.Store) error {
	itemChan := make(chan *migrationItem)
	errChan := make(chan error)
	defer close(itemChan)
	go func() {
		for item := range itemChan {
			item.ns = stateStoreNamespace
			if err := s.Put(item); err != nil {
				errChan <- err
				return
			}
		}
	}()
	return s.Iterate(storage.Query{
		Factory: func() storage.Item { return new(migrationItem) },
	}, func(res storage.Result) (stop bool, err error) {
		if strings.HasPrefix(res.ID, stateStoreNamespace) {
			return false, nil
		}
		item := res.Entry.(*migrationItem)
		item.key = res.ID
		select {
		case itemChan <- item:
			return false, nil
		case err := <-errChan:
			return true, err
		}
	})
}

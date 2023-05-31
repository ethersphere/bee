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

func epochMigration(s storage.Store) error {
	itemChan := make(chan *rawItem)
	errChan := make(chan error)
	done := make(chan struct{})

	go func() {
		defer close(done)

		for item := range itemChan {
			item.ns = stateStoreNamespace
			if err := s.Put(item); err != nil {
				errChan <- err
				return
			}
		}
	}()
	err := s.Iterate(storage.Query{
		Factory: func() storage.Item { return &rawItem{newItemProxy("", []byte(nil))} },
	}, func(res storage.Result) (stop bool, err error) {
		if strings.HasPrefix(res.ID, stateStoreNamespace) {
			return false, nil
		}
		item := res.Entry.(*rawItem)
		item.key = res.ID
		select {
		case itemChan <- item:
			return false, nil
		case err := <-errChan:
			return true, err
		}
	})
	if err != nil {
		return err
	}
	close(itemChan)
	<-done

	return nil
}

package migration

import (
	"fmt"
	"os"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
)

// step_05 is a migration step that removes all upload items from the store.
func step_05(st storage.BatchedStore) error {
	logger := log.NewLogger("migration-step-05", log.WithSink(os.Stdout))
	logger.Info("start removing upload items")

	var itemsToDelete []storage.Item
	err := upload.IterateAll(st, func(u storage.Item) (bool, error) {
		itemsToDelete = append(itemsToDelete, u)
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("iterate upload items: %w", err)
	}

	for _, item := range itemsToDelete {
		err := st.Delete(item)
		if err != nil {
			return fmt.Errorf("delete upload item: %w", err)
		}
	}
	logger.Info("finished removing upload items")
	return nil
}

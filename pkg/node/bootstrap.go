package node

import (
	"github.com/ethersphere/bee/v2/pkg/storage"
)

func batchStoreExists(s storage.StateStorer) (bool, error) {
	hasOne := false
	err := s.Iterate("batchstore_", func(key, value []byte) (stop bool, err error) {
		hasOne = true
		return true, err
	})

	return hasOne, err
}

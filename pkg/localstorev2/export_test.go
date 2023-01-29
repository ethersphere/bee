package localstore

import storage "github.com/ethersphere/bee/pkg/storagev2"

func (db *DB) Repo() *storage.Repository {
	return db.repo
}

func ReplaceSharkyShardLimit(val int) {
	sharkyNoOfShards = val
}

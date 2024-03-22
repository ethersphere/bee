package kvs

import (
	"errors"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/kvs/manifest"
	"github.com/ethersphere/bee/pkg/kvs/memory"
	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrInvalidKvsType = errors.New("kvs: invalid type")

type KeyValueStore interface {
	Get(rootHash swarm.Address, key []byte) ([]byte, error)
	Put(rootHash swarm.Address, key, value []byte) (swarm.Address, error)
}

// func NewDefaultKeyValueStore(storer api.Storer) (KeyValueStore, error) {
// 	return NewKeyValueStore(storer, memory.KvsTypeMemory)
// }

func NewKeyValueStore(storer api.Storer, kvsType string) (KeyValueStore, error) {
	switch kvsType {
	case "":
		return memory.NewMemoryKeyValueStore()
	case memory.KvsTypeMemory:
		return memory.NewMemoryKeyValueStore()
	case manifest.KvsTypeManifest:
		return manifest.NewManifestKeyValueStore(storer)
	default:
		return nil, ErrInvalidKvsType
	}
}

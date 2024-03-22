package manifest

import (
	"context"
	"encoding/hex"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/file/redundancy"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	// KvsTypeManifest represents
	KvsTypeManifest = "Manifest"
)

type ManifestKeyValueStore interface {
	Get(rootHash swarm.Address, key []byte) ([]byte, error)
	Put(rootHash swarm.Address, key, value []byte) (swarm.Address, error)
}

type manifestKeyValueStore struct {
	storer api.Storer
}

// TODO: pass context as dep.
func (m *manifestKeyValueStore) Get(rootHash swarm.Address, key []byte) ([]byte, error) {
	ls := loadsave.NewReadonly(m.storer.ChunkStore())
	// existing manif
	manif, err := manifest.NewSimpleManifestReference(rootHash, ls)
	if err != nil {
		return nil, err
	}
	entry, err := manif.Lookup(context.Background(), hex.EncodeToString(key))
	if err != nil {
		return nil, err
	}
	ref := entry.Reference()
	return ref.Bytes(), nil
}

func (m *manifestKeyValueStore) Put(rootHash swarm.Address, key []byte, value []byte) (swarm.Address, error) {
	factory := requestPipelineFactory(context.Background(), m.storer.Cache(), false, redundancy.NONE)
	ls := loadsave.New(m.storer.ChunkStore(), m.storer.Cache(), factory)
	// existing manif
	manif, err := manifest.NewSimpleManifestReference(rootHash, ls)
	if err != nil {
		// new manif
		manif, err = manifest.NewSimpleManifest(ls)
		if err != nil {
			return swarm.EmptyAddress, err
		}
	}
	err = manif.Add(context.Background(), hex.EncodeToString(key), manifest.NewEntry(swarm.NewAddress(value), map[string]string{}))
	if err != nil {
		return swarm.EmptyAddress, err
	}
	manifRef, err := manif.Store(context.Background())
	if err != nil {
		return swarm.EmptyAddress, err
	}

	putter := m.storer.DirectUpload()
	err = putter.Done(manifRef)
	if err != nil {
		return swarm.EmptyAddress, err
	}
	return manifRef, nil
}

func NewManifestKeyValueStore(storer api.Storer) (ManifestKeyValueStore, error) {
	return &manifestKeyValueStore{
		storer: storer,
	}, nil
}

func requestPipelineFactory(ctx context.Context, s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
	}
}

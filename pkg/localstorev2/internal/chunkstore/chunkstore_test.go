package chunkstore_test

import (
	"math"
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestRetrievalIndexItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &chunkstore.RetrievalIndexItem{},
			Factory:    func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			MarshalErr: chunkstore.ErrInvalidRetrievalIndexItemAddress,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address: swarm.ZeroAddress,
			},
			Factory:    func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			MarshalErr: chunkstore.ErrInvalidRetrievalIndexItemAddress,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				Timestamp: math.MaxUint64,
				Location: sharky.Location{
					Shard:  math.MaxUint8,
					Slot:   math.MaxUint32,
					Length: math.MaxUint16,
				},
			},
			Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			UnmarshalErr: chunkstore.ErrInvalidRetrievalIndexItemSize,
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

func TestChunkStampItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	minAddress := swarm.NewAddress(storagetest.MinAddressBytes[:])
	minStamp := postage.NewStamp(make([]byte, 32), make([]byte, 8), make([]byte, 8), make([]byte, 65))
	chunk := chunktesting.GenerateRandomTestChunk()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &chunkstore.ChunkStampItem{},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidChunkStampItemAddress,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: swarm.ZeroAddress,
			},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidChunkStampItemAddress,
		},
	}, {
		name: "nil stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: minAddress,
			},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidChunkStampItemStamp,
		},
	}, {
		name: "zero stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: minAddress,
				Stamp:   new(postage.Stamp),
			},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidChunkStampItemStamp,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: minAddress,
				Stamp:   minStamp,
			},
			Factory: func() storage.Item { return &chunkstore.ChunkStampItem{Address: minAddress} },
		},
	}, {
		name: "valid values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: chunk.Address(),
				Stamp:   chunk.Stamp(),
			},
			Factory: func() storage.Item { return &chunkstore.ChunkStampItem{Address: chunk.Address()} },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(chunkstore.ChunkStampItem) },
			UnmarshalErr: chunkstore.ErrInvalidChunkStampItemSize,
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

package sampler_test

import (
	"hash"
	"sort"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmtpool"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storageincentives/sampler"
	"github.com/ethersphere/bee/pkg/swarm"
)

// RandSample returns Sample with random values.
func RandSample(anchor []byte) (sampler.Data, error) {
	chunks := make([]swarm.Chunk, sampler.Size)
	for i := 0; i < sampler.Size; i++ {
		ch := chunk.GenerateTestRandomChunk()
		// if i%3 == 0 {
		// 	ch = chunk.GenerateTestRandomSoChunk(t, ch)
		// }
		chunks[i] = ch
	}

	return MakeSampleUsingChunks(chunks, anchor)
}

// MakeSampleUsingChunks returns Sample constructed using supplied chunks.
func MakeSampleUsingChunks(chunks []swarm.Chunk, anchor []byte) (sampler.Data, error) {
	prefixHasherFactory := func() hash.Hash {
		return swarm.NewPrefixHasher(anchor)
	}
	items := make([]sampler.Item, len(chunks))
	for i, ch := range chunks {
		// these provers need to be used for proofs later and assumed are in hashed state
		// also the hasher taken from bmtpool musst not be put back
		prover := bmt.Prover{Hasher: bmtpool.Get()}
		transProver := bmt.Prover{Hasher: bmt.NewHasher(prefixHasherFactory)}
		tr, err := sampler.TransformedAddressCAC(transProver, ch)
		if err != nil {
			return sampler.Data{}, err
		}

		items[i] = sampler.Item{
			Address:      ch.Address(),
			TransAddress: tr,
			Prover:       prover,
			TransProver:  transProver,
			Stamp:        sampler.NewStamp(ch.Stamp()),
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].TransAddress.Compare(items[j].TransAddress) == -1
	})

	return sampler.Data{Items: items}, nil
}

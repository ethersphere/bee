package storageincentives

import (
	"bytes"
	"testing"

	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSampleChunk(t *testing.T) {
	t.Parallel()

	sample := storer.RandSampleT(t)

	chunk, err := sampleChunk(sample.Items)
	if err != nil {
		t.Fatal(err)
	}

	data := chunk.Data()[swarm.SpanSize:]
	pos := 0
	for _, item := range sample.Items {
		if !bytes.Equal(data[pos:pos+swarm.HashSize], item.ChunkAddress.Bytes()) {
			t.Error("expected chunk address")
		}
		pos += swarm.HashSize

		if !bytes.Equal(data[pos:pos+swarm.HashSize], item.TransformedAddress.Bytes()) {
			t.Error("expected transformed address")
		}
		pos += swarm.HashSize
	}

	if swarm.ZeroAddress.Equal(chunk.Address()) || swarm.EmptyAddress.Equal(chunk.Address()) {
		t.Error("hash should not be empty or zero")
	}
}

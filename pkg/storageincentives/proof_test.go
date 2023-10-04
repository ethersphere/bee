// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/pkg/storageincentives"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Tests asserts that creating sample chunk is valid for all lengths [1-MaxSampleSize]
func TestSampleChunk(t *testing.T) {
	t.Parallel()

	sample := storer.RandSample(t, nil)

	for i := 1; i < len(sample.Items); i++ {
		items := sample.Items[:i]

		chunk, err := storageincentives.SampleChunk(items)
		if err != nil {
			t.Fatal(err)
		}

		data := chunk.Data()[swarm.SpanSize:]
		pos := 0
		for _, item := range items {
			if !bytes.Equal(data[pos:pos+swarm.HashSize], item.ChunkAddress.Bytes()) {
				t.Error("expected chunk address")
			}
			pos += swarm.HashSize

			if !bytes.Equal(data[pos:pos+swarm.HashSize], item.TransformedAddress.Bytes()) {
				t.Error("expected transformed address")
			}
			pos += swarm.HashSize
		}
	}
}

// Tests asserts that creating sample chunk should fail because it will exceed
// capacity of chunk data.
func TestSampleChunkExpectedError(t *testing.T) {
	t.Parallel()

	sampleItem := storer.RandSample(t, nil).Items[0]

	items := make([]storer.SampleItem, 65)
	for i := range items {
		items[i] = sampleItem
	}

	_, err := storageincentives.SampleChunk(items)
	if err == nil {
		t.Fatal("expecting error")
	}
}

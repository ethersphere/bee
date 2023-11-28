// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tools

import (
	"context"

	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/bmt"
	enc "github.com/ethersphere/bee/pkg/file/pipeline/encryption"
	"github.com/ethersphere/bee/pkg/file/pipeline/hashtrie"
	"github.com/ethersphere/bee/pkg/file/pipeline/store"
	"github.com/ethersphere/bee/pkg/file/redundancy"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// NewErasureHashTrieWriter returns back an redundancy param and a HastTrieWriter pipeline
// which are using simple BMT and StoreWriter pipelines for chunk writes
func NewErasureHashTrieWriter(
	ctx context.Context,
	s storage.Putter,
	rLevel redundancy.Level,
	encryptChunks bool,
	intermediateChunkPipeline, parityChunkPipeline pipeline.ChainWriter,
) (redundancy.IParams, pipeline.ChainWriter) {
	pf := func() pipeline.ChainWriter {
		lsw := store.NewStoreWriter(ctx, s, intermediateChunkPipeline)
		return bmt.NewBmtWriter(lsw)
	}
	if encryptChunks {
		pf = func() pipeline.ChainWriter {
			lsw := store.NewStoreWriter(ctx, s, intermediateChunkPipeline)
			b := bmt.NewBmtWriter(lsw)
			return enc.NewEncryptionWriter(encryption.NewChunkEncrypter(), b)
		}
	}
	ppf := func() pipeline.ChainWriter {
		lsw := store.NewStoreWriter(ctx, s, parityChunkPipeline)
		return bmt.NewBmtWriter(lsw)
	}

	hashSize := swarm.HashSize
	if encryptChunks {
		hashSize *= 2
	}

	r := redundancy.New(rLevel, encryptChunks, ppf)
	ht := hashtrie.NewHashTrieWriter(hashSize, r, pf)
	return r, ht
}

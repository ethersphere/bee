// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import (
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var ErrCorrupted = errors.New("chunk store: corrupted chunk")

// Verify returns the type of a loaded chunk, or ErrCorrupted if the load
// failed or the chunk is neither CAC nor SOC.
func Verify(ch swarm.Chunk, loadErr error) (swarm.ChunkType, error) {
	if loadErr != nil {
		return swarm.ChunkTypeUnspecified, fmt.Errorf("%w: %w", ErrCorrupted, loadErr)
	}
	chunkType := storage.ChunkType(ch)
	if chunkType == swarm.ChunkTypeUnspecified {
		return swarm.ChunkTypeUnspecified, fmt.Errorf("%w: %w", ErrCorrupted, storage.ErrInvalidChunk)
	}
	return chunkType, nil
}

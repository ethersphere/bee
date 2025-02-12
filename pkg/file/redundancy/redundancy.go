// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

// ParityChunkCallback is called when a new parity chunk has been created
type ParityChunkCallback func(level int, span, address []byte) error

type RedundancyParams interface {
	MaxShards() int // returns the maximum data shard number being used in an intermediate chunk
	Level() Level
	Parities(int) int
	ChunkWrite(int, []byte, ParityChunkCallback) error
	ElevateCarrierChunk(int, ParityChunkCallback) error
	Encode(int, ParityChunkCallback) error
	GetRootData() ([]byte, error)
}

type ErasureEncoder interface {
	Encode([][]byte) error
}

var erasureEncoderFunc = func(shards, parities int) (ErasureEncoder, error) {
	return reedsolomon.New(shards, parities)
}

type Params struct {
	level      Level
	pipeLine   pipeline.PipelineFunc
	buffer     [][][]byte // keeps bytes of chunks on each level for producing erasure coded data; [levelIndex][branchIndex][byteIndex]
	cursor     []int      // index of the current buffered chunk in Buffer. this is basically the latest used branchIndex.
	maxShards  int        // number of chunks after which the parity encode function should be called
	maxParity  int        // number of parity chunks if maxShards has been reached for erasure coding
	encryption bool
}

func New(level Level, encryption bool, pipeLine pipeline.PipelineFunc) *Params {
	maxShards := 0
	maxParity := 0
	if encryption {
		maxShards = level.GetMaxEncShards()
		maxParity = level.GetParities(swarm.EncryptedBranches)
	} else {
		maxShards = level.GetMaxShards()
		maxParity = level.GetParities(swarm.BmtBranches)
	}
	// init dataBuffer for erasure coding
	rsChunkLevels := 0
	if level != NONE {
		rsChunkLevels = 8
	}
	Buffer := make([][][]byte, rsChunkLevels)
	for i := 0; i < rsChunkLevels; i++ {
		Buffer[i] = make([][]byte, swarm.BmtBranches) // 128 long always because buffer varies at encrypted chunks
	}

	return &Params{
		level:      level,
		pipeLine:   pipeLine,
		buffer:     Buffer,
		cursor:     make([]int, 9),
		maxShards:  maxShards,
		maxParity:  maxParity,
		encryption: encryption,
	}
}

func (p *Params) MaxShards() int {
	return p.maxShards
}

func (p *Params) Level() Level {
	return p.level
}

func (p *Params) Parities(shards int) int {
	if p.encryption {
		return p.level.GetEncParities(shards)
	}
	return p.level.GetParities(shards)
}

// ChunkWrite caches the chunk data on the given chunk level and if it is full then it calls Encode
func (p *Params) ChunkWrite(chunkLevel int, data []byte, callback ParityChunkCallback) error {
	if p.level == NONE {
		return nil
	}
	if len(data) != swarm.ChunkWithSpanSize {
		zeros := make([]byte, swarm.ChunkWithSpanSize-len(data))
		data = append(data, zeros...)
	}

	return p.chunkWrite(chunkLevel, data, callback)
}

// ChunkWrite caches the chunk data on the given chunk level and if it is full then it calls Encode
func (p *Params) chunkWrite(chunkLevel int, data []byte, callback ParityChunkCallback) error {
	// append chunk to the buffer
	p.buffer[chunkLevel][p.cursor[chunkLevel]] = data
	p.cursor[chunkLevel]++

	// add parity chunk if it is necessary
	if p.cursor[chunkLevel] == p.maxShards {
		// append erasure coded data
		return p.encode(chunkLevel, callback)
	}
	return nil
}

// Encode produces and stores parity chunks that will be also passed back to the caller
func (p *Params) Encode(chunkLevel int, callback ParityChunkCallback) error {
	if p.level == NONE || p.cursor[chunkLevel] == 0 {
		return nil
	}

	return p.encode(chunkLevel, callback)
}

func (p *Params) encode(chunkLevel int, callback ParityChunkCallback) error {
	shards := p.cursor[chunkLevel]
	parities := p.Parities(shards)

	n := shards + parities
	// realloc for parity chunks if it does not override the prev one
	// calculate parity chunks
	enc, err := erasureEncoderFunc(shards, parities)
	if err != nil {
		return err
	}

	pz := len(p.buffer[chunkLevel][0])
	for i := shards; i < n; i++ {
		p.buffer[chunkLevel][i] = make([]byte, pz)
	}
	err = enc.Encode(p.buffer[chunkLevel][:n])
	if err != nil {
		return err
	}

	for i := shards; i < n; i++ {
		chunkData := p.buffer[chunkLevel][i]
		span := chunkData[:swarm.SpanSize]

		writer := p.pipeLine()
		args := pipeline.PipeWriteArgs{
			Data: chunkData,
			Span: span,
		}
		err = writer.ChainWrite(&args)
		if err != nil {
			return err
		}

		err = callback(chunkLevel+1, span, args.Ref)
		if err != nil {
			return err
		}
	}
	p.cursor[chunkLevel] = 0

	return nil
}

// ElevateCarrierChunk moves the last poor orphan chunk to the level above where it can fit and there are other chunks as well.
func (p *Params) ElevateCarrierChunk(chunkLevel int, callback ParityChunkCallback) error {
	if p.level == NONE {
		return nil
	}
	if p.cursor[chunkLevel] != 1 {
		return fmt.Errorf("redundancy: cannot elevate carrier chunk because it is not the only chunk on the level. It has %d chunks", p.cursor[chunkLevel])
	}

	// not necessary to update current level since we will not work with it anymore
	return p.chunkWrite(chunkLevel+1, p.buffer[chunkLevel][p.cursor[chunkLevel]-1], callback)
}

// GetRootData returns the topmost chunk in the tree.
// throws and error if the encoding has not been finished in the BMT
// OR redundancy is not used in the BMT
func (p *Params) GetRootData() ([]byte, error) {
	if p.level == NONE {
		return nil, fmt.Errorf("redundancy: no redundancy level is used for the file in order to cache root data")
	}
	lastBuffer := p.buffer[len(p.buffer)-1]
	if len(lastBuffer[0]) != swarm.ChunkWithSpanSize {
		return nil, fmt.Errorf("redundancy: hashtrie sum has not finished in order to cache root data")
	}
	return lastBuffer[0], nil
}
